package models

import "time"

type Namespace struct {
	Name     string             `json:"name"  validate:"required,hostname_rfc1123,excludes=.,lowercase"`
	Owner    string             `json:"owner"`
	TenantID string             `json:"tenant_id"`
	Members  []Member           `json:"members"`
	Settings *NamespaceSettings `json:"settings"`
	Devices  int                `json:"-"`

	DevicesAcceptedCount int64 `json:"devices_accepted_count"`
	DevicesPendingCount  int64 `json:"devices_pending_count"`
	DevicesRejectedCount int64 `json:"devices_rejected_count"`
	DevicesRemovedCount  int64 `json:"devices_removed_count"`

	Sessions   int       `json:"-"`
	MaxDevices int       `json:"max_devices"`
	CreatedAt  time.Time `json:"created_at"`
	Billing    *Billing  `json:"billing"`
	Type       Type      `json:"type"`
}

// HasMaxDevices checks if the namespace has a maximum number of devices.
//
// Generally, a namespace has a MaxDevices value greater than 0 when the ShellHub is either in community version or
// the namespace does not have a billing plan enabled, because, in this case, we set this value to -1.
func (n *Namespace) HasMaxDevices() bool {
	return n.MaxDevices > 0
}

// HasMaxDevicesReached checks if the namespace has reached the maximum number of devices.
// Only counts accepted devices. Removed devices no longer count towards the limit,
// allowing immediate slot reuse after deletion.
func (n *Namespace) HasMaxDevicesReached() bool {
	return n.DevicesAcceptedCount >= int64(n.MaxDevices)
}

// FindMember checks if a member with the specified ID exists in the namespace.
func (n *Namespace) FindMember(id string) (*Member, bool) {
	for _, member := range n.Members {
		if member.ID == id {
			return &member, true
		}
	}

	return nil, false
}

type NamespaceSettings struct {
	SessionRecord          bool   `json:"session_record"`
	ConnectionAnnouncement string `json:"connection_announcement"`
	DeviceAutoAccept       bool   `json:"device_auto_accept"`
	AllowPassword          bool   `json:"allow_password"`
	AllowPublicKey         bool   `json:"allow_public_key"`
	AllowRoot              bool   `json:"allow_root"`
	AllowEmptyPasswords    bool   `json:"allow_empty_passwords"`
	AllowTTY               bool   `json:"allow_tty"`
	AllowTCPForwarding     bool   `json:"allow_tcp_forwarding"`
	AllowWebEndpoints      bool   `json:"allow_web_endpoints"`
	AllowSFTP              bool   `json:"allow_sftp"`
	AllowAgentForwarding   bool   `json:"allow_agent_forwarding"`
}

// DefaultNamespaceSettings returns the permissive defaults applied by the namespace_settings
// migration, used when a namespace's settings row hasn't been created yet.
func DefaultNamespaceSettings() *NamespaceSettings {
	return &NamespaceSettings{
		SessionRecord:        true,
		AllowPassword:        true,
		AllowPublicKey:       true,
		AllowRoot:            true,
		AllowEmptyPasswords:  true,
		AllowTTY:             true,
		AllowTCPForwarding:   true,
		AllowWebEndpoints:    true,
		AllowSFTP:            true,
		AllowAgentForwarding: true,
	}
}

// NamespaceSettingsPatch holds a partial update to a namespace's settings. Only non-nil
// fields are applied, allowing the store layer to update exactly the requested columns in
// a single statement instead of writing back a full settings row read earlier by the caller.
type NamespaceSettingsPatch struct {
	SessionRecord          *bool
	ConnectionAnnouncement *string
	DeviceAutoAccept       *bool
	AllowPassword          *bool
	AllowPublicKey         *bool
	AllowRoot              *bool
	AllowEmptyPasswords    *bool
	AllowTTY               *bool
	AllowTCPForwarding     *bool
	AllowWebEndpoints      *bool
	AllowSFTP              *bool
	AllowAgentForwarding   *bool
}

// IsEmpty reports whether the patch has no fields set, i.e. applying it would be a no-op.
func (p *NamespaceSettingsPatch) IsEmpty() bool {
	return p.SessionRecord == nil &&
		p.ConnectionAnnouncement == nil &&
		p.DeviceAutoAccept == nil &&
		p.AllowPassword == nil &&
		p.AllowPublicKey == nil &&
		p.AllowRoot == nil &&
		p.AllowEmptyPasswords == nil &&
		p.AllowTTY == nil &&
		p.AllowTCPForwarding == nil &&
		p.AllowWebEndpoints == nil &&
		p.AllowSFTP == nil &&
		p.AllowAgentForwarding == nil
}

// default Announcement Message for the shellhub namespace
const DefaultAnnouncementMessage = `
******************************************************************
*                                                                *
*             Welcome to ShellHub Community Edition!             *
*                                                                *
* ShellHub is a next-generation SSH server, providing a          *
* seamless, secure, and user-friendly solution for remote        *
* access management. With ShellHub, you can manage all your      *
* devices effortlessly from a single platform, ensuring optimal  *
* security and productivity.                                     *
*                                                                *
* Want to learn more about ShellHub and explore other editions?  *
* Visit: https://shellhub.io                                     *
*                                                                *
* Join our community and contribute to our open-source project:  *
* https://github.com/shellhub-io/shellhub                        *
*                                                                *
* For assistance, please contact the system administrator.       *
*                                                                *
******************************************************************
`

// NamespaceConflicts holds namespace attributes that must be unique for each document and can be utilized in queries
// to identify conflicts.
type NamespaceConflicts struct {
	Name string
}

// Distinct removes the c attributes whether it's equal to the namespace attribute.
func (c *NamespaceConflicts) Distinct(namespace *Namespace) {
	if c.Name == namespace.Name {
		c.Name = ""
	}
}
