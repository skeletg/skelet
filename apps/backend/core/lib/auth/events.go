package auth

import "github.com/google/uuid"

type Subject string // discriminates between auth flows - plugins filter on it so a customer-facing 2FA plugin doesn't try to step up an admin login

const (
	SubjectAdmin    Subject = "admin"
	SubjectCustomer Subject = "customer"
)

const (
	EventLoginAttempting  = "auth.login.attempting"
	EventLoginSucceeded   = "auth.login.succeeded"
	EventLoginFailed      = "auth.login.failed"
	EventLogout           = "auth.logout"
	EventPasswordResetReq = "auth.password.reset.requesting"
	EventPasswordReset    = "auth.password.reset.completed"
	EventRefreshIssued    = "auth.refresh.issued"
	EventRefreshRevoked   = "auth.refresh.revoked"
)

// Fires BEFORE password verification - sync handlers may abort the login for rate-limit, IP-block or geo-anomaly reasons, or write through Require2FA to force a step-up the auth handler then enforces
type LoginAttempting struct {
	Subject    Subject
	Email      string
	IP         string
	UserAgent  string
	ChannelID  uuid.UUID // uuid.Nil for admin logins, since admin auth isn't store-scoped - customer logins always carry the storefront's channel
	Require2FA *bool     // mutable
}

func (*LoginAttempting) Type() string                { return EventLoginAttempting }
func (e *LoginAttempting) EventChannelID() uuid.UUID { return e.ChannelID }

type LoginSucceeded struct { // fires AFTER password verification and the account-active checks pass - async
	Subject   Subject
	SubjectID uuid.UUID // admin_user.id or customer.id
	Email     string
	IP        string
	UserAgent string
	ChannelID uuid.UUID
}

func (*LoginSucceeded) Type() string                { return EventLoginSucceeded }
func (e *LoginSucceeded) EventChannelID() uuid.UUID { return e.ChannelID }

type LoginFailed struct {
	Subject   Subject
	Email     string
	IP        string
	UserAgent string
	ChannelID uuid.UUID
	Reason    string // "invalid_credentials" | "account_disabled" | other
}

func (*LoginFailed) Type() string                { return EventLoginFailed }
func (e *LoginFailed) EventChannelID() uuid.UUID { return e.ChannelID }

type Logout struct { // fires AFTER one session is revoked by its own logout - async; revoke-all publishes RefreshRevoked instead, so AllDevices stays false and is kept for the webhook payload
	Subject    Subject
	SubjectID  uuid.UUID
	AllDevices bool
}

func (*Logout) Type() string { return EventLogout }

// Fires BEFORE a reset token is minted, sync - a plugin can reject, e.g. SSO-only accounts that can't reset through the email flow
type PasswordResetRequesting struct {
	Subject   Subject
	Email     string
	IP        string
	ChannelID uuid.UUID
}

func (*PasswordResetRequesting) Type() string                { return EventPasswordResetReq }
func (e *PasswordResetRequesting) EventChannelID() uuid.UUID { return e.ChannelID }

type PasswordResetCompleted struct { // fires AFTER a password is changed through the reset token - async
	Subject   Subject
	SubjectID uuid.UUID
	Email     string
	ChannelID uuid.UUID
}

func (*PasswordResetCompleted) Type() string                { return EventPasswordReset }
func (e *PasswordResetCompleted) EventChannelID() uuid.UUID { return e.ChannelID }

type RefreshIssued struct {
	Subject   Subject
	SubjectID uuid.UUID
	SessionID uuid.UUID
	IP        string
	UserAgent string
}

func (*RefreshIssued) Type() string { return EventRefreshIssued }

type RefreshRevoked struct {
	Subject   Subject
	SubjectID uuid.UUID
	SessionID uuid.UUID
	Reason    string
}

func (*RefreshRevoked) Type() string { return EventRefreshRevoked }
