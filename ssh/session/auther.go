package session

import (
	"context"
	"crypto/x509"
	"encoding/pem"

	"github.com/Masterminds/semver"
	gliderssh "github.com/gliderlabs/ssh"
	"github.com/shellhub-io/shellhub/ssh/pkg/magickey"
	gossh "golang.org/x/crypto/ssh"
)

type authFunc func(*Session, *gossh.ClientConfig) error

type authMethod int8

const (
	AuthMethodPublicKey authMethod = iota // AuthMethodPassword represents a public key authentication
	AuthMethodPassword                    // AuthMethodPassword represents a password authentication
)

// Auth interface defines a common interface for authenticating a session. An 'Auth'
// must have an associated [authMethod], an [authFunc] to authenticate the session, and
// an 'Evaluate' method to evaluate the session's context if necessary (e.g. the agent
// version when authenticating with public keys).
type Auth interface {
	// Method returns the associated authentication method.
	Method() authMethod

	// Auth defines the callback that must be called when authenticating the session.
	Auth() authFunc

	// Evaluate evaluates the session's context, returning an error if there's something
	// possibly broken. It's not always necessary.
	Evaluate(*Session) error
}

// settingDisallowed reports whether an SSH capability is disallowed, checking the
// namespace's setting first and then the device's override. A nil pointer means that
// level (namespace or device settings not loaded) imposes no restriction.
func settingDisallowed(namespaceAllowed, deviceAllowed *bool) bool {
	if namespaceAllowed != nil && !*namespaceAllowed {
		return true
	}

	return deviceAllowed != nil && !*deviceAllowed
}

type publicKeyAuth struct {
	pk gliderssh.PublicKey
}

func AuthPublicKey(pk gliderssh.PublicKey) Auth {
	return &publicKeyAuth{pk: pk}
}

func (*publicKeyAuth) Method() authMethod {
	return AuthMethodPublicKey
}

func (*publicKeyAuth) Auth() authFunc {
	return func(session *Session, config *gossh.ClientConfig) error {
		privateKey, err := session.api.CreatePrivateKey(context.TODO())
		if err != nil {
			return err
		}

		block, _ := pem.Decode(privateKey.Data)

		parsed, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return err
		}

		signer, err := gossh.NewSignerFromKey(parsed)
		if err != nil {
			return err
		}

		config.Auth = []gossh.AuthMethod{
			gossh.PublicKeys(signer),
		}

		return nil
	}
}

func (p *publicKeyAuth) Evaluate(session *Session) error {
	var namespaceAllowPublicKey, deviceAllowPublicKey *bool
	if session.Namespace.Settings != nil {
		namespaceAllowPublicKey = &session.Namespace.Settings.AllowPublicKey
	}
	if session.Device != nil && session.Device.SSH != nil {
		deviceAllowPublicKey = &session.Device.SSH.AllowPublicKey
	}
	if settingDisallowed(namespaceAllowPublicKey, deviceAllowPublicKey) {
		return ErrPublicKeyDisabled
	}

	if session.Target.Username == "root" {
		var namespaceAllowRoot, deviceAllowRoot *bool
		if session.Namespace.Settings != nil {
			namespaceAllowRoot = &session.Namespace.Settings.AllowRoot
		}
		if session.Device != nil && session.Device.SSH != nil {
			deviceAllowRoot = &session.Device.SSH.AllowRoot
		}
		if settingDisallowed(namespaceAllowRoot, deviceAllowRoot) {
			return ErrRootDisabled
		}
	}

	// Versions earlier than 0.6.0 do not validate the user when receiving a public key
	// authentication request. This implies that requests with invalid users are
	// treated as "authenticated" because the connection does not raise any error.
	// Moreover, the agent panics after the connection ends. To avoid this, connections
	// with public key are not permitted when agent version is 0.5.x or earlier.
	if !sshconf.AllowPublickeyAccessBelow060 {
		version := session.Device.Info.Version
		if version != "latest" {
			semverVersion, err := semver.NewVersion(version)
			if err != nil {
				return ErrInvalidVersion
			}

			if semverVersion.LessThan(semver.MustParse("0.6.0")) {
				return ErrUnsuportedPublicKeyAuth
			}
		}
	}

	fingerprint := gossh.FingerprintLegacyMD5(p.pk)

	magic, err := gossh.NewPublicKey(&magickey.GetReference().PublicKey)
	if err != nil {
		return err
	}

	if gossh.FingerprintLegacyMD5(magic) != fingerprint {
		if _, err = session.api.GetPublicKey(context.TODO(), fingerprint, session.Device.TenantID); err != nil {
			return err
		}

		if ok, err := session.api.EvaluateKey(context.TODO(), fingerprint, session.Device, session.Data.Target.Username); !ok || err != nil {
			return ErrEvaluatePublicKey
		}
	}

	return err
}

type passwordAuth struct {
	pwd string
}

func AuthPassword(pwd string) Auth {
	return &passwordAuth{pwd: pwd}
}

func (*passwordAuth) Method() authMethod {
	return AuthMethodPassword
}

func (p *passwordAuth) Auth() authFunc {
	return func(_ *Session, config *gossh.ClientConfig) error {
		config.Auth = []gossh.AuthMethod{
			gossh.Password(p.pwd),
		}

		return nil
	}
}

func (p *passwordAuth) Evaluate(session *Session) error {
	var namespaceAllowPassword, deviceAllowPassword *bool
	if session.Namespace.Settings != nil {
		namespaceAllowPassword = &session.Namespace.Settings.AllowPassword
	}
	if session.Device != nil && session.Device.SSH != nil {
		deviceAllowPassword = &session.Device.SSH.AllowPassword
	}
	if settingDisallowed(namespaceAllowPassword, deviceAllowPassword) {
		return ErrPasswordDisabled
	}

	if session.Target.Username == "root" {
		var namespaceAllowRoot, deviceAllowRoot *bool
		if session.Namespace.Settings != nil {
			namespaceAllowRoot = &session.Namespace.Settings.AllowRoot
		}
		if session.Device != nil && session.Device.SSH != nil {
			deviceAllowRoot = &session.Device.SSH.AllowRoot
		}
		if settingDisallowed(namespaceAllowRoot, deviceAllowRoot) {
			return ErrRootDisabled
		}
	}

	if p.pwd == "" {
		var namespaceAllowEmpty, deviceAllowEmpty *bool
		if session.Namespace.Settings != nil {
			namespaceAllowEmpty = &session.Namespace.Settings.AllowEmptyPasswords
		}
		if session.Device != nil && session.Device.SSH != nil {
			deviceAllowEmpty = &session.Device.SSH.AllowEmptyPasswords
		}
		if settingDisallowed(namespaceAllowEmpty, deviceAllowEmpty) {
			return ErrEmptyPasswordNotPermitted
		}
	}

	return nil
}
