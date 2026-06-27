// Package sctp implements the V3 SCTP listener for the SSH service.
// It accepts raw SCTP connections from agents, validates their device token
// via the internal API, and registers the connection in the dialer Manager.
package sctp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ishidawataru/sctp"
	"github.com/shellhub-io/shellhub/pkg/api/internalclient"
	"github.com/shellhub-io/shellhub/pkg/sctpadapter"
	"github.com/shellhub-io/shellhub/ssh/pkg/dialer"
	log "github.com/sirupsen/logrus"
)

const authReadTimeout = 10 * time.Second

// authPayload is the first message sent by the agent on the SCTP connection.
type authPayload struct {
	Token string `json:"token"`
}

// Server accepts SCTP connections from V3 agents.
type Server struct {
	addr   string
	dialer *dialer.Dialer
	client internalclient.Client
}

// New creates a new SCTP server that binds to addr and registers accepted
// connections in d.
func New(addr string, d *dialer.Dialer, cli internalclient.Client) *Server {
	return &Server{addr: addr, dialer: d, client: cli}
}

// ListenAndServe starts the SCTP listener and blocks until it fails.
func (s *Server) ListenAndServe() error {
	laddr, err := sctp.ResolveSCTPAddr("sctp", s.addr)
	if err != nil {
		return fmt.Errorf("sctp: resolve %s: %w", s.addr, err)
	}

	ln, err := sctp.ListenSCTPExt(
		"sctp",
		laddr,
		sctp.InitMsg{
			NumOstreams:  sctpadapter.MaxStreams,
			MaxInstreams: sctpadapter.MaxStreams,
		},
	)
	if err != nil {
		return fmt.Errorf("sctp: listen %s: %w", s.addr, err)
	}

	log.WithField("addr", s.addr).Info("SCTP server listening")

	for {
		conn, err := ln.AcceptSCTP()
		if err != nil {
			log.WithError(err).Error("sctp: accept failed")

			continue
		}

		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn *sctp.SCTPConn) {
	logger := log.WithField("remote", conn.RemoteAddr())

	defer func() {
		if r := recover(); r != nil {
			logger.WithField("panic", r).Error("sctp: panic in handleConn")
			conn.Close()
		}
	}()

	conn.SetDeadline(time.Now().Add(authReadTimeout)) //nolint:errcheck

	var payload authPayload

	dec := json.NewDecoder(conn)
	if err := dec.Decode(&payload); err != nil {
		logger.WithError(err).Warn("sctp: failed to decode auth payload")
		conn.Close()

		return
	}

	conn.SetDeadline(time.Time{}) //nolint:errcheck

	uid, tenantID, err := s.client.ValidateDeviceToken(context.Background(), payload.Token)
	if err != nil || uid == "" {
		logger.WithError(err).Warn("sctp: token validation failed")
		conn.Close()

		return
	}

	if _, err := conn.Write([]byte(`{"status":"ok"}`)); err != nil {
		logger.WithError(err).Warn("sctp: failed to send auth ack")
		conn.Close()

		return
	}

	mux := sctpadapter.NewMux(conn)

	if err := s.dialer.Manager.BindSCTP(tenantID, uid, mux); err != nil {
		logger.WithError(err).Error("sctp: failed to bind connection")
		mux.Close()

		return
	}

	logger.WithFields(log.Fields{
		"uid":       uid,
		"tenant_id": tenantID,
	}).Info("sctp: agent registered (v3)")
}

