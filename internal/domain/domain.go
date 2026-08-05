// Package domain holds the entities shared by every layer of the panel.
package domain

import (
	"encoding/json"
	"time"
)

type AdminRole string

const (
	RoleOwner  AdminRole = "OWNER"
	RoleAdmin  AdminRole = "ADMIN"
	RoleViewer AdminRole = "VIEWER"
)

func (r AdminRole) Valid() bool {
	switch r {
	case RoleOwner, RoleAdmin, RoleViewer:
		return true
	}
	return false
}

// CanWrite reports whether the role may mutate panel state.
func (r AdminRole) CanWrite() bool { return r == RoleOwner || r == RoleAdmin }

type Admin struct {
	UUID         string     `json:"uuid"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"-"`
	Role         AdminRole  `json:"role"`
	IsDisabled   bool       `json:"isDisabled"`
	LastLoginAt  *time.Time `json:"lastLoginAt"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`

	// Two-factor. The secret and the recovery digests never leave the panel —
	// TOTPEnabled is all the UI needs, and anything more would put a working
	// second factor into every response that carries an administrator.
	TOTPSecret         string     `json:"-"`
	TOTPEnabled        bool       `json:"totpEnabled"`
	TOTPConfirmedAt    *time.Time `json:"totpConfirmedAt"`
	TOTPLastStep       int64      `json:"-"`
	RecoveryCodeHashes []string   `json:"-"`

	// A count, never the codes: an operator needs to know they are running low,
	// and nothing more can be given back once they have been shown.
	RecoveryCodesLeft int `json:"recoveryCodesLeft"`
}

// ConfigProfile is a reusable xray-core configuration document. Nodes are bound
// to exactly one profile and serve a subset of its inbounds.
type ConfigProfile struct {
	UUID      string          `json:"uuid"`
	Name      string          `json:"name"`
	Config    json.RawMessage `json:"config"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`

	Inbounds []ConfigProfileInbound `json:"inbounds,omitempty"`
	NodeIDs  []string               `json:"nodeUuids,omitempty"`
}

// ConfigProfileInbound mirrors one inbound of the profile document. It is
// extracted on save so squads and hosts can reference inbounds by identity
// instead of by fragile string tags.
type ConfigProfileInbound struct {
	UUID            string `json:"uuid"`
	ConfigProfileID string `json:"profileUuid"`
	ProfileName     string `json:"profileName,omitempty"`
	Tag             string `json:"tag"`
	Type            string `json:"type"`
	Network         string `json:"network"`
	Security        string `json:"security"`
	Port            int    `json:"port"`
}

// Settings holds the panel-wide knobs an operator can change at runtime,
// including white-label branding for corporate deployments.
type Settings struct {
	BrandName         string `json:"brandName"`
	BrandTagline      string `json:"brandTagline"`
	BrandLogo         string `json:"brandLogo"`
	BrandAccent       string `json:"brandAccent"`
	SubscriptionTitle string `json:"subscriptionTitle"`
	SupportURL        string `json:"supportUrl"`
	Currency          string `json:"currency"`
	// SubscriptionFormat is what an unrecognised client is served. Empty keeps
	// the base64 list, which every client can read. Clients that announce
	// themselves — Clash, sing-box — still get their own format regardless.
	SubscriptionFormat string `json:"subscriptionFormat"`
	// What the subscriber's page offers. Both default to on, so an existing
	// deployment looks exactly as it did.
	SubPageShowLinks   bool `json:"subPageShowLinks"`
	SubPageShowFormats bool `json:"subPageShowFormats"`
	// Custom documents for the two formats that are whole config files rather
	// than a list of links. Empty means the built-in template.
	ClashTemplate   string `json:"clashTemplate"`
	SingBoxTemplate string `json:"singboxTemplate"`
	// RequireTOTP refuses a session to any administrator without a second
	// factor, sending them to enrolment instead.
	RequireTOTP bool `json:"requireTotp"`
	// How early to warn. Zero disables either warning, which is what every
	// install did before these existed.
	WarnExpiryDays   int       `json:"warnExpiryDays"`
	WarnQuotaPercent int       `json:"warnQuotaPercent"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// BillingCycle is how often a node has to be paid for.
type BillingCycle string

const (
	BillingNone      BillingCycle = "NONE"
	BillingMonthly   BillingCycle = "MONTHLY"
	BillingQuarterly BillingCycle = "QUARTERLY"
	BillingYearly    BillingCycle = "YEARLY"
)

func (c BillingCycle) Valid() bool {
	switch c {
	case BillingNone, BillingMonthly, BillingQuarterly, BillingYearly:
		return true
	}
	return false
}

// MonthlyCost normalises a billing cycle onto a monthly figure so nodes on
// different cycles can be summed.
func (c BillingCycle) MonthlyCost(amount float64) float64 {
	switch c {
	case BillingMonthly:
		return amount
	case BillingQuarterly:
		return amount / 3
	case BillingYearly:
		return amount / 12
	default:
		return 0
	}
}

// Advance returns the next payment date one cycle after the given one.
func (c BillingCycle) Advance(from time.Time) time.Time {
	switch c {
	case BillingMonthly:
		return from.AddDate(0, 1, 0)
	case BillingQuarterly:
		return from.AddDate(0, 3, 0)
	case BillingYearly:
		return from.AddDate(1, 0, 0)
	default:
		return from
	}
}

// Device is one client seen on a user's subscription.
type Device struct {
	UserID string `json:"userUuid"`
	// Username is filled by the cross-user inspector so the list is readable
	// without a lookup per row.
	Username  string    `json:"username,omitempty"`
	HWID      string    `json:"hwid"`
	UserAgent string    `json:"userAgent"`
	Platform  string    `json:"platform"`
	FirstSeen time.Time `json:"firstSeen"`
	LastSeen  time.Time `json:"lastSeen"`
}

// APIToken authenticates an external integration against the panel API.
type APIToken struct {
	UUID       string     `json:"uuid"`
	Name       string     `json:"name"`
	TokenHash  string     `json:"-"`
	Preview    string     `json:"tokenPreview"`
	CreatedBy  string     `json:"createdBy"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	ExpiresAt  *time.Time `json:"expiresAt"`
	CreatedAt  time.Time  `json:"createdAt"`
}

type NodeHealth string

const (
	NodeHealthUnknown      NodeHealth = "UNKNOWN"
	NodeHealthConnecting   NodeHealth = "CONNECTING"
	NodeHealthOnline       NodeHealth = "ONLINE"
	NodeHealthDegraded     NodeHealth = "DEGRADED"
	NodeHealthOffline      NodeHealth = "OFFLINE"
	NodeHealthDisabled     NodeHealth = "DISABLED"
	NodeHealthTrafficLimit NodeHealth = "TRAFFIC_LIMITED"
)

type TrafficResetStrategy string

const (
	ResetNever TrafficResetStrategy = "NO_RESET"
	ResetDay   TrafficResetStrategy = "DAY"
	ResetWeek  TrafficResetStrategy = "WEEK"
	ResetMonth TrafficResetStrategy = "MONTH"
)

func (s TrafficResetStrategy) Valid() bool {
	switch s {
	case ResetNever, ResetDay, ResetWeek, ResetMonth:
		return true
	}
	return false
}

// Node is a server running the agent and an xray-core process.
type Node struct {
	UUID        string `json:"uuid"`
	Name        string `json:"name"`
	Address     string `json:"address"`
	CountryCode string `json:"countryCode"`
	Description string `json:"description"`

	TokenHash string `json:"-"`
	// TokenPreview keeps the last characters of the enrolment token so the UI can
	// tell two nodes apart without ever exposing the secret again.
	TokenPreview string `json:"tokenPreview"`

	IsDisabled bool       `json:"isDisabled"`
	Health     NodeHealth `json:"health"`

	ConfigProfileID    *string              `json:"configProfileUuid"`
	ConfigProfileName  string               `json:"configProfileName,omitempty"`
	ActiveInboundTags  []string             `json:"activeInboundTags"`
	ConsumptionMultip  float64              `json:"consumptionMultiplier"`
	TrafficLimitBytes  int64                `json:"trafficLimitBytes"`
	TrafficUsedBytes   int64                `json:"trafficUsedBytes"`
	TrafficResetPolicy TrafficResetStrategy `json:"trafficResetStrategy"`
	NotifyPercent      int                  `json:"notifyPercent"`
	LastTrafficResetAt *time.Time           `json:"lastTrafficResetAt"`

	AgentVersion  string     `json:"agentVersion"`
	XrayVersion   string     `json:"xrayVersion"`
	XrayRunning   bool       `json:"xrayRunning"`
	XrayStartedAt *time.Time `json:"xrayStartedAt"`
	ConfigHash    string     `json:"configHash"`

	Hostname      string  `json:"hostname"`
	OS            string  `json:"os"`
	Arch          string  `json:"arch"`
	Kernel        string  `json:"kernel"`
	CPUCount      int     `json:"cpuCount"`
	CPUModel      string  `json:"cpuModel"`
	CPUUsage      float64 `json:"cpuUsagePercent"`
	TotalRAMBytes int64   `json:"totalRamBytes"`
	UsedRAMBytes  int64   `json:"usedRamBytes"`
	LoadAvg1      float64 `json:"loadAvg1"`
	OnlineUsers   int     `json:"onlineUsers"`

	// Infrastructure billing: what this node costs and who it is rented from.
	Provider      string       `json:"provider"`
	ProviderURL   string       `json:"providerUrl"`
	CostAmount    float64      `json:"costAmount"`
	CostCurrency  string       `json:"costCurrency"`
	BillingCycle  BillingCycle `json:"billingCycle"`
	NextPaymentAt *time.Time   `json:"nextPaymentAt"`
	BillingNotes  string       `json:"billingNotes"`
	Tags          []string     `json:"tags"`

	StatusMessage   string     `json:"statusMessage"`
	LastStatusAt    *time.Time `json:"lastStatusChangeAt"`
	LastConnectedAt *time.Time `json:"lastConnectedAt"`
	ViewPosition    int        `json:"viewPosition"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Host is a client-facing entry point advertised in subscriptions. It points at
// one inbound of a config profile but may carry its own address/SNI/etc so a
// single inbound can be published behind many domains or CDNs.
type Host struct {
	UUID        string `json:"uuid"`
	InboundID   string `json:"inboundUuid"`
	InboundTag  string `json:"inboundTag,omitempty"`
	InboundType string `json:"inboundType,omitempty"`
	ProfileID   string `json:"configProfileUuid,omitempty"`
	ProfileName string `json:"configProfileName,omitempty"`

	Remark  string `json:"remark"`
	Address string `json:"address"`
	Port    int    `json:"port"`

	Path          string   `json:"path"`
	SNI           string   `json:"sni"`
	HostHeader    string   `json:"hostHeader"`
	ALPN          string   `json:"alpn"`
	Fingerprint   string   `json:"fingerprint"`
	PublicKey     string   `json:"publicKey"`
	ShortID       string   `json:"shortId"`
	SpiderX       string   `json:"spiderX"`
	Flow          string   `json:"flow"`
	Security      string   `json:"security"`
	AllowInsecure bool     `json:"allowInsecure"`
	Tags          []string `json:"tags"`

	IsDisabled   bool `json:"isDisabled"`
	ViewPosition int  `json:"viewPosition"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Squad groups inbounds together; users are attached to squads rather than to
// individual inbounds, which keeps bulk membership changes cheap.
type Squad struct {
	UUID      string    `json:"uuid"`
	Name      string    `json:"name"`
	Info      string    `json:"info"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	InboundIDs  []string               `json:"inboundUuids"`
	Inbounds    []ConfigProfileInbound `json:"inbounds,omitempty"`
	MemberCount int                    `json:"membersCount"`
}

type UserStatus string

const (
	UserActive   UserStatus = "ACTIVE"
	UserDisabled UserStatus = "DISABLED"
	UserLimited  UserStatus = "LIMITED"
	UserExpired  UserStatus = "EXPIRED"
)

func (s UserStatus) Valid() bool {
	switch s {
	case UserActive, UserDisabled, UserLimited, UserExpired:
		return true
	}
	return false
}

// User is a VPN client.
type User struct {
	UUID      string `json:"uuid"`
	ShortUUID string `json:"shortUuid"`
	Username  string `json:"username"`

	SubscriptionUUID string `json:"subscriptionUuid"`
	VlessUUID        string `json:"vlessUuid"`
	TrojanPassword   string `json:"trojanPassword"`
	SSPassword       string `json:"ssPassword"`

	Status               UserStatus           `json:"status"`
	TrafficLimitBytes    int64                `json:"trafficLimitBytes"`
	UsedTrafficBytes     int64                `json:"usedTrafficBytes"`
	LifetimeTrafficBytes int64                `json:"lifetimeUsedTrafficBytes"`
	TrafficResetPolicy   TrafficResetStrategy `json:"trafficLimitStrategy"`
	LastTrafficResetAt   *time.Time           `json:"lastTrafficResetAt"`

	ExpireAt *time.Time `json:"expireAt"`
	OnlineAt *time.Time `json:"onlineAt"`

	Description string `json:"description"`
	Tag         string `json:"tag"`
	Email       string `json:"email"`
	TelegramID  *int64 `json:"telegramId"`

	HWIDDeviceLimit int        `json:"hwidDeviceLimit"`
	SubLastUA       string     `json:"subLastUserAgent"`
	SubLastOpenedAt *time.Time `json:"subLastOpenedAt"`
	SubRevokedAt    *time.Time `json:"subRevokedAt"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	SquadIDs []string `json:"squadUuids"`
	Squads   []Squad  `json:"squads,omitempty"`
}

// XrayEmail is the identity xray-core reports usage under. Keeping the uuid in
// front guarantees uniqueness even when usernames are recycled.
func (u *User) XrayEmail() string { return u.UUID + "." + u.Username }

// IsUsable reports whether the user should currently be provisioned to nodes.
func (u *User) IsUsable() bool { return u.Status == UserActive }

// NodeUsageSample is one bucketed traffic measurement for a node.
type NodeUsageSample struct {
	NodeID   string    `json:"nodeUuid"`
	NodeName string    `json:"nodeName,omitempty"`
	At       time.Time `json:"at"`
	Bytes    int64     `json:"bytes"`
}

// UserUsageSample is one bucketed traffic measurement for a user.
type UserUsageSample struct {
	UserID string    `json:"userUuid"`
	NodeID string    `json:"nodeUuid"`
	At     time.Time `json:"at"`
	Bytes  int64     `json:"bytes"`
}

// EventKind classifies audit log records.
type EventKind string

const (
	EventNodeConnected    EventKind = "NODE_CONNECTED"
	EventNodeDisconnected EventKind = "NODE_DISCONNECTED"
	EventNodeConfigPushed EventKind = "NODE_CONFIG_PUSHED"
	EventNodeError        EventKind = "NODE_ERROR"
	EventUserCreated      EventKind = "USER_CREATED"
	EventUserUpdated      EventKind = "USER_UPDATED"
	EventUserDeleted      EventKind = "USER_DELETED"
	EventUserLimited      EventKind = "USER_LIMITED"
	EventUserExpired      EventKind = "USER_EXPIRED"
	// Sent before the fact rather than after, so an operator or a billing
	// system can act while the subscriber still has service.
	EventUserExpiringSoon EventKind = "USER_EXPIRING_SOON"
	EventUserQuotaWarning EventKind = "USER_QUOTA_WARNING"
	EventAdminLogin       EventKind = "ADMIN_LOGIN"
	EventAdminLoginFailed EventKind = "ADMIN_LOGIN_FAILED"
	// A second factor being turned on or off, and a sign-in locked for
	// repeated failures — both are worth waking someone up for.
	EventAdminSecurity   EventKind = "ADMIN_SECURITY"
	EventAdminLocked     EventKind = "ADMIN_LOCKED"
	EventProfileUpdated  EventKind = "PROFILE_UPDATED"
	EventNodePaymentDue  EventKind = "NODE_PAYMENT_DUE"
	EventDeviceBlocked   EventKind = "DEVICE_LIMIT_REACHED"
	EventSettingsUpdated EventKind = "SETTINGS_UPDATED"
)

// AllEventKinds is the list a channel picks its subscriptions from, and the
// list the UI renders. Keeping it beside the constants means a new event kind
// is subscribable the moment it exists.
var AllEventKinds = []EventKind{
	EventNodeConnected, EventNodeDisconnected, EventNodeConfigPushed, EventNodeError,
	EventUserCreated, EventUserUpdated, EventUserDeleted, EventUserLimited, EventUserExpired,
	EventUserExpiringSoon, EventUserQuotaWarning,
	EventAdminLogin, EventAdminLoginFailed, EventAdminSecurity, EventAdminLocked,
	EventProfileUpdated, EventNodePaymentDue,
	EventDeviceBlocked, EventSettingsUpdated,
}

func (k EventKind) Valid() bool {
	for _, known := range AllEventKinds {
		if k == known {
			return true
		}
	}
	return false
}

// ChannelKind is how a notification leaves the panel.
type ChannelKind string

const (
	ChannelWebhook  ChannelKind = "WEBHOOK"
	ChannelTelegram ChannelKind = "TELEGRAM"
)

func (k ChannelKind) Valid() bool {
	return k == ChannelWebhook || k == ChannelTelegram
}

// NotificationChannel is one destination events are delivered to. Config holds
// the transport-specific fields — a URL and signing secret, or a bot token and
// chat id — so adding a transport does not mean adding columns.
type NotificationChannel struct {
	UUID   string          `json:"uuid"`
	Name   string          `json:"name"`
	Kind   ChannelKind     `json:"kind"`
	Config json.RawMessage `json:"config"`
	// Empty subscribes to everything, which is what most deployments want and
	// avoids a channel that silently delivers nothing.
	Events    []EventKind `json:"events"`
	IsEnabled bool        `json:"isEnabled"`

	LastOK     *bool      `json:"lastOk"`
	LastDetail string     `json:"lastDetail"`
	LastSentAt *time.Time `json:"lastSentAt"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Wants reports whether this channel should receive an event.
func (c NotificationChannel) Wants(kind EventKind) bool {
	if !c.IsEnabled {
		return false
	}
	if len(c.Events) == 0 {
		return true
	}
	for _, want := range c.Events {
		if want == kind {
			return true
		}
	}
	return false
}

// NotificationDelivery is one attempt to reach a channel, kept so "the webhook
// never arrived" is an answerable question.
type NotificationDelivery struct {
	ID         int64     `json:"id"`
	ChannelID  string    `json:"channelUuid"`
	EventKind  EventKind `json:"eventKind"`
	OK         bool      `json:"ok"`
	Detail     string    `json:"detail"`
	Attempts   int       `json:"attempts"`
	DurationMS int       `json:"durationMs"`
	CreatedAt  time.Time `json:"createdAt"`
}

// NodeMetric is one heartbeat sample, kept so load can be read as a trend.
type NodeMetric struct {
	At          time.Time `json:"at"`
	CPUPercent  float64   `json:"cpuPercent"`
	UsedRAM     int64     `json:"usedRamBytes"`
	TotalRAM    int64     `json:"totalRamBytes"`
	LoadAvg1    float64   `json:"loadAvg1"`
	OnlineUsers int       `json:"onlineUsers"`
}

// AnnouncementLevel tints the notice on the subscription page.
type AnnouncementLevel string

const (
	AnnouncementInfo    AnnouncementLevel = "INFO"
	AnnouncementWarning AnnouncementLevel = "WARNING"
	AnnouncementDanger  AnnouncementLevel = "DANGER"
)

func (l AnnouncementLevel) Valid() bool {
	switch l {
	case AnnouncementInfo, AnnouncementWarning, AnnouncementDanger:
		return true
	}
	return false
}

// Announcement is a notice shown to subscribers on their subscription page.
type Announcement struct {
	UUID      string            `json:"uuid"`
	Title     string            `json:"title"`
	Body      string            `json:"body"`
	Level     AnnouncementLevel `json:"level"`
	IsEnabled bool              `json:"isEnabled"`
	StartsAt  *time.Time        `json:"startsAt"`
	EndsAt    *time.Time        `json:"endsAt"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

// Live reports whether the announcement should be shown at the given moment.
// A window with no bounds is always live, which is the common case.
func (a Announcement) Live(now time.Time) bool {
	if !a.IsEnabled {
		return false
	}
	if a.StartsAt != nil && now.Before(*a.StartsAt) {
		return false
	}
	if a.EndsAt != nil && now.After(*a.EndsAt) {
		return false
	}
	return true
}

type Event struct {
	ID        int64           `json:"id"`
	Kind      EventKind       `json:"kind"`
	Actor     string          `json:"actor"`
	Subject   string          `json:"subject"`
	Message   string          `json:"message"`
	Meta      json.RawMessage `json:"meta,omitempty"`
	CreatedAt time.Time       `json:"createdAt"`
}
