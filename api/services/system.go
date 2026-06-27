package services

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/shellhub-io/shellhub/api/pkg/responses"
	"github.com/shellhub-io/shellhub/pkg/api/requests"
	"github.com/shellhub-io/shellhub/pkg/envs"
)

type SystemService interface {
	// GetSystemInfo retrieves the instance's information
	GetSystemInfo(ctx context.Context, req *requests.GetSystemInfo) (*responses.SystemInfo, error)

	SystemDownloadInstallScript(ctx context.Context) (string, error)
}

func (s *service) GetSystemInfo(ctx context.Context, req *requests.GetSystemInfo) (*responses.SystemInfo, error) {
	system, err := s.store.SystemGet(ctx)
	if err != nil {
		return nil, err
	}

	apiHost := strings.Split(req.Host, ":")[0]
	sshPort := envs.DefaultBackend.Get("SHELLHUB_SSH_PORT")
	sctpPort := envs.DefaultBackend.Get("SHELLHUB_SCTP_PORT")

	// SHELLHUB_SCTP_HOST overrides the host used in the SCTP endpoint. If the
	// value is a hostname (e.g. a Docker service name), it is resolved to an IP
	// so that agents can reach the SSH container directly without going through
	// any userspace port proxy.
	sctpHost := apiHost
	if override := envs.DefaultBackend.Get("SHELLHUB_SCTP_HOST"); override != "" {
		if addrs, err := net.LookupHost(override); err == nil && len(addrs) > 0 {
			sctpHost = addrs[0]
		} else {
			sctpHost = override
		}
	}

	resp := &responses.SystemInfo{
		Version: envs.DefaultBackend.Get("SHELLHUB_VERSION"),
		Setup:   system.Setup,
		Endpoints: &responses.SystemEndpointsInfo{
			API:  apiHost,
			SSH:  fmt.Sprintf("%s:%s", apiHost, sshPort),
			SCTP: fmt.Sprintf("%s:%s", sctpHost, sctpPort),
		},
		Authentication: &responses.SystemAuthenticationInfo{
			Local: system.Authentication.Local.Enabled,
		},
	}

	if req.Port > 0 {
		resp.Endpoints.API = fmt.Sprintf("%s:%d", apiHost, req.Port)
	} else {
		resp.Endpoints.API = req.Host
	}

	return resp, nil
}

func (s *service) SystemDownloadInstallScript(_ context.Context) (string, error) {
	data, err := os.ReadFile("/templates/install.sh")
	if err != nil {
		return "", err
	}

	return string(data), nil
}
