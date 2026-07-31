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
	EventAdminLogin       EventKind = "ADMIN_LOGIN"
	EventAdminLoginFailed EventKind = "ADMIN_LOGIN_FAILED"
	EventProfileUpdated   EventKind = "PROFILE_UPDATED"
)

type Event struct {
	ID        int64           `json:"id"`
	Kind      EventKind       `json:"kind"`
	Actor     string          `json:"actor"`
	Subject   string          `json:"subject"`
	Message   string          `json:"message"`
	Meta      json.RawMessage `json:"meta,omitempty"`
	CreatedAt time.Time       `json:"createdAt"`
}
