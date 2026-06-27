// Package sctpadapter provides a stream-multiplexing layer over a single
// SCTP connection using the kernel's native per-stream delivery. It replaces
// what yamux does over a WebSocket, but delegates ordering, reliability and
// framing to the SCTP transport.
//
// A [Mux] wraps one *sctp.SCTPConn and exposes:
//   - [Mux] as a net.Listener (agent side): Accept blocks until the remote
//     peer opens a new stream, then returns a net.Conn for that stream.
//   - [Mux.OpenStream]: opens a stream toward the remote peer (server side).
package sctpadapter

import (
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ishidawataru/sctp"
)

const (
	socketBufSize = 4 * 1024 * 1024
	// sackDelayMS=0 disables delayed-SACK: the kernel sends a SACK after
	// every received DATA chunk. Maximum CWND feedback for the sender at the
	// cost of a few extra small packets — right trade-off for local bulk transfer.
	sackDelayMS = 0
	// recvChanSize is the number of SCTP chunks buffered per stream. Each
	// chunk is up to readBufSize (64 KiB), so this caps per-stream receive
	// memory at 512 × 64 KiB = 32 MiB — enough to keep the SCTP window full
	// without stalling the shared readLoop on a slow consumer.
	recvChanSize = 512
)

// TuneConn applies throughput-oriented socket options to an SCTP connection.
func TuneConn(conn *sctp.SCTPConn) {
	one := int32(1)
	conn.Setsockopt(sctp.SCTP_NODELAY, uintptr(unsafe.Pointer(&one)), unsafe.Sizeof(one)) //nolint:errcheck

	conn.SetWriteBuffer(socketBufSize) //nolint:errcheck
	conn.SetReadBuffer(socketBufSize)  //nolint:errcheck

	conn.SetSackTimer(&sctp.SackTimer{SackDelay: sackDelayMS}) //nolint:errcheck
}

// singleAddr normalises a multi-homed SCTP address (e.g.
// "1.2.3.4/5.6.7.8:5222") to a plain "host:port" form.
func singleAddr(addr net.Addr) net.Addr {
	s := addr.String()
	if idx := strings.IndexByte(s, '/'); idx != -1 {
		if colon := strings.LastIndexByte(s, ':'); colon != -1 {
			s = s[:idx] + s[colon:]
		} else {
			s = s[:idx]
		}
	}

	return &singleTCPAddr{s}
}

type singleTCPAddr struct{ s string }

func (a *singleTCPAddr) Network() string { return "sctp" }
func (a *singleTCPAddr) String() string  { return a.s }

const (
	readBufSize = 65536
	MaxStreams   = 1024
)

var ErrMuxClosed = errors.New("sctpadapter: mux closed")

// streamConn is a virtual net.Conn backed by one SCTP stream ID.
//
// Received data is queued in a buffered channel (recv) by the shared
// readLoop goroutine. This means the readLoop never blocks on a slow
// consumer — the SCTP socket stays drained and kernel flow-control does
// not throttle the remote sender.
//
// Write calls SCTPWrite directly on the underlying connection.
type streamConn struct {
	id       uint16
	mux      *Mux
	recv     chan []byte // owned by readLoop (writer) and Read caller (reader)
	leftover []byte     // partial remainder from last Read; only touched by Read caller
	once     sync.Once
	done     chan struct{}
	closeErr atomic.Value // stores error, set before closing done
}

func newStreamConn(id uint16, m *Mux) *streamConn {
	return &streamConn{
		id:   id,
		mux:  m,
		recv: make(chan []byte, recvChanSize),
		done: make(chan struct{}),
	}
}

func (s *streamConn) Read(b []byte) (int, error) {
	// Serve leftover bytes from a previous partial read first.
	if len(s.leftover) > 0 {
		n := copy(b, s.leftover)
		s.leftover = s.leftover[n:]

		return n, nil
	}

	select {
	case chunk, ok := <-s.recv:
		if !ok {
			return 0, io.EOF
		}

		n := copy(b, chunk)
		if n < len(chunk) {
			s.leftover = chunk[n:]
		}

		return n, nil
	case <-s.done:
		// Drain one pending chunk before reporting EOF so callers get any
		// data that arrived before the close signal.
		select {
		case chunk, ok := <-s.recv:
			if ok {
				n := copy(b, chunk)
				if n < len(chunk) {
					s.leftover = chunk[n:]
				}

				return n, nil
			}
		default:
		}

		if v := s.closeErr.Load(); v != nil {
			return 0, v.(error)
		}

		return 0, io.EOF
	}
}

func (s *streamConn) Write(b []byte) (int, error) {
	return s.mux.conn.SCTPWrite(b, &sctp.SndRcvInfo{Stream: s.id})
}

func (s *streamConn) Close() error {
	s.closeWithErr(nil)

	return nil
}

func (s *streamConn) closeWithErr(err error) {
	s.once.Do(func() {
		if err != nil {
			s.closeErr.Store(err)
		}

		close(s.done)
	})
}

func (s *streamConn) LocalAddr() net.Addr  { return singleAddr(s.mux.conn.LocalAddr()) }
func (s *streamConn) RemoteAddr() net.Addr { return singleAddr(s.mux.conn.RemoteAddr()) }

func (s *streamConn) SetDeadline(_ time.Time) error      { return nil }
func (s *streamConn) SetReadDeadline(_ time.Time) error  { return nil }
func (s *streamConn) SetWriteDeadline(_ time.Time) error { return nil }

// Mux multiplexes multiple logical streams over a single *sctp.SCTPConn.
// It implements net.Listener so it can be passed directly to tunnel.TunnelV2.Listen.
type Mux struct {
	conn      *sctp.SCTPConn
	streams   sync.Map        // uint16 → *streamConn
	accept    chan *streamConn
	nextID    atomic.Uint32
	closed    atomic.Bool
	closeOnce sync.Once
	done      chan struct{}
}

// NewMux wraps conn and starts the internal read loop.
func NewMux(conn *sctp.SCTPConn) *Mux {
	TuneConn(conn)
	conn.SubscribeEvents(sctp.SCTP_EVENT_DATA_IO) //nolint:errcheck

	m := &Mux{
		conn:   conn,
		accept: make(chan *streamConn, 256),
		done:   make(chan struct{}),
	}

	go m.readLoop()

	return m
}

func (m *Mux) readLoop() {
	defer close(m.done)

	buf := make([]byte, readBufSize)

	for {
		n, info, err := m.conn.SCTPRead(buf)
		if err != nil {
			m.closeAllStreams(err)

			return
		}

		if n == 0 {
			continue
		}

		var id uint16
		if info != nil {
			id = info.Stream
		}

		val, loaded := m.streams.LoadOrStore(id, newStreamConn(id, m))
		sc := val.(*streamConn)

		if !loaded && !m.closed.Load() {
			select {
			case m.accept <- sc:
			default:
			}
		}

		// Copy before enqueue: buf is reused on the next SCTPRead.
		chunk := make([]byte, n)
		copy(chunk, buf[:n])

		// Non-blocking send: if the per-stream buffer is full, fall back to a
		// blocking send so backpressure is applied rather than dropping data.
		select {
		case sc.recv <- chunk:
		default:
			select {
			case sc.recv <- chunk:
			case <-sc.done:
				// Stream was closed; discard the chunk.
			}
		}
	}
}

func (m *Mux) closeAllStreams(err error) {
	m.streams.Range(func(_, val any) bool {
		val.(*streamConn).closeWithErr(err)

		return true
	})
}

// OpenStream opens a new outbound stream with an auto-incremented ID.
func (m *Mux) OpenStream() (net.Conn, error) {
	if m.closed.Load() {
		return nil, ErrMuxClosed
	}

	id := uint16(m.nextID.Add(1))
	sc := newStreamConn(id, m)
	m.streams.Store(id, sc)

	return sc, nil
}

// Accept implements net.Listener.
func (m *Mux) Accept() (net.Conn, error) {
	select {
	case sc, ok := <-m.accept:
		if !ok {
			return nil, ErrMuxClosed
		}

		return sc, nil
	case <-m.done:
		return nil, ErrMuxClosed
	}
}

// Addr implements net.Listener.
func (m *Mux) Addr() net.Addr { return m.conn.LocalAddr() }

// Close implements net.Listener.
func (m *Mux) Close() error {
	var err error

	m.closeOnce.Do(func() {
		m.closed.Store(true)
		err = m.conn.Close()
	})

	return err
}

// Done returns a channel that is closed when the underlying connection closes.
func (m *Mux) Done() <-chan struct{} { return m.done }
