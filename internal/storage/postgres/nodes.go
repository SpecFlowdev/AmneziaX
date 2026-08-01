package postgres

import (
	"context"
	"time"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
	"github.com/google/uuid"
)

const nodeColumns = `n.uuid, n.name, n.address, n.country_code, n.description, n.token_hash, n.token_preview,
	n.is_disabled, n.health, n.config_profile_uuid, COALESCE(p.name, ''), n.active_inbound_tags,
	n.consumption_multiplier, n.traffic_limit_bytes, n.traffic_used_bytes, n.traffic_reset_strategy,
	n.notify_percent, n.last_traffic_reset_at, n.agent_version, n.xray_version, n.xray_running,
	n.xray_started_at, n.config_hash, n.hostname, n.os, n.arch, n.kernel, n.cpu_count, n.cpu_model,
	n.cpu_usage, n.total_ram_bytes, n.used_ram_bytes, n.load_avg_1, n.online_users, n.status_message,
	n.last_status_at, n.last_connected_at, n.view_position, n.created_at, n.updated_at,
	n.provider, n.provider_url, n.cost_amount, n.cost_currency, n.billing_cycle,
	n.next_payment_at, n.billing_notes, n.tags`

const nodeFrom = ` FROM nodes n LEFT JOIN config_profiles p ON p.uuid = n.config_profile_uuid`

func scanNode(row interface{ Scan(...any) error }) (*domain.Node, error) {
	var n domain.Node
	err := row.Scan(&n.UUID, &n.Name, &n.Address, &n.CountryCode, &n.Description, &n.TokenHash, &n.TokenPreview,
		&n.IsDisabled, &n.Health, &n.ConfigProfileID, &n.ConfigProfileName, &n.ActiveInboundTags,
		&n.ConsumptionMultip, &n.TrafficLimitBytes, &n.TrafficUsedBytes, &n.TrafficResetPolicy,
		&n.NotifyPercent, &n.LastTrafficResetAt, &n.AgentVersion, &n.XrayVersion, &n.XrayRunning,
		&n.XrayStartedAt, &n.ConfigHash, &n.Hostname, &n.OS, &n.Arch, &n.Kernel, &n.CPUCount, &n.CPUModel,
		&n.CPUUsage, &n.TotalRAMBytes, &n.UsedRAMBytes, &n.LoadAvg1, &n.OnlineUsers, &n.StatusMessage,
		&n.LastStatusAt, &n.LastConnectedAt, &n.ViewPosition, &n.CreatedAt, &n.UpdatedAt,
		&n.Provider, &n.ProviderURL, &n.CostAmount, &n.CostCurrency, &n.BillingCycle,
		&n.NextPaymentAt, &n.BillingNotes, &n.Tags)
	if err != nil {
		return nil, mapErr(err)
	}
	if n.ActiveInboundTags == nil {
		n.ActiveInboundTags = []string{}
	}
	if n.Tags == nil {
		n.Tags = []string{}
	}
	return &n, nil
}

type NodeInput struct {
	Name              string
	Address           string
	CountryCode       string
	Description       string
	IsDisabled        bool
	ConfigProfileID   *string
	ActiveInboundTags []string
	Consumption       float64
	TrafficLimitBytes int64
	TrafficReset      domain.TrafficResetStrategy
	NotifyPercent     int
	ViewPosition      int

	Provider      string
	ProviderURL   string
	CostAmount    float64
	CostCurrency  string
	BillingCycle  domain.BillingCycle
	NextPaymentAt *time.Time
	BillingNotes  string
	Tags          []string
}

func (s *Store) CreateNode(ctx context.Context, in NodeInput, tokenHash, tokenPreview string) (*domain.Node, error) {
	id := uuid.NewString()
	_, err := s.pool.Exec(ctx, `INSERT INTO nodes
		(uuid, name, address, country_code, description, token_hash, token_preview, is_disabled,
		 config_profile_uuid, active_inbound_tags, consumption_multiplier, traffic_limit_bytes,
		 traffic_reset_strategy, notify_percent, view_position, health,
		 provider, provider_url, cost_amount, cost_currency, billing_cycle, next_payment_at,
		 billing_notes, tags)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)`,
		id, in.Name, in.Address, in.CountryCode, in.Description, tokenHash, tokenPreview, in.IsDisabled,
		in.ConfigProfileID, in.ActiveInboundTags, in.Consumption, in.TrafficLimitBytes,
		in.TrafficReset, in.NotifyPercent, in.ViewPosition, domain.NodeHealthUnknown,
		in.Provider, in.ProviderURL, in.CostAmount, in.CostCurrency, in.BillingCycle,
		in.NextPaymentAt, in.BillingNotes, in.Tags)
	if err != nil {
		return nil, mapErr(err)
	}
	return s.Node(ctx, id)
}

func (s *Store) UpdateNode(ctx context.Context, id string, in NodeInput) (*domain.Node, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE nodes SET name=$2, address=$3, country_code=$4, description=$5,
		is_disabled=$6, config_profile_uuid=$7, active_inbound_tags=$8, consumption_multiplier=$9,
		traffic_limit_bytes=$10, traffic_reset_strategy=$11, notify_percent=$12, view_position=$13,
		provider=$14, provider_url=$15, cost_amount=$16, cost_currency=$17, billing_cycle=$18,
		next_payment_at=$19, billing_notes=$20, tags=$21, updated_at=NOW()
		WHERE uuid = $1`,
		id, in.Name, in.Address, in.CountryCode, in.Description, in.IsDisabled, in.ConfigProfileID,
		in.ActiveInboundTags, in.Consumption, in.TrafficLimitBytes, in.TrafficReset, in.NotifyPercent,
		in.ViewPosition, in.Provider, in.ProviderURL, in.CostAmount, in.CostCurrency, in.BillingCycle,
		in.NextPaymentAt, in.BillingNotes, in.Tags)
	if err != nil {
		return nil, mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return s.Node(ctx, id)
}

func (s *Store) Node(ctx context.Context, id string) (*domain.Node, error) {
	return scanNode(s.pool.QueryRow(ctx, `SELECT `+nodeColumns+nodeFrom+` WHERE n.uuid = $1`, id))
}

func (s *Store) ListNodes(ctx context.Context) ([]domain.Node, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+nodeColumns+nodeFrom+` ORDER BY n.view_position, n.created_at`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []domain.Node{}
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, mapErr(rows.Err())
}

// NodesUsingProfile lists the nodes bound to a profile, used to fan out config
// changes after the profile document is edited.
func (s *Store) NodesUsingProfile(ctx context.Context, profileID string) ([]domain.Node, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+nodeColumns+nodeFrom+` WHERE n.config_profile_uuid = $1`, profileID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []domain.Node{}
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, mapErr(rows.Err())
}

func (s *Store) DeleteNode(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM nodes WHERE uuid = $1`, id)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RotateNodeToken(ctx context.Context, id, tokenHash, tokenPreview string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE nodes SET token_hash=$2, token_preview=$3, updated_at=NOW() WHERE uuid=$1`,
		id, tokenHash, tokenPreview)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetNodeDisabled(ctx context.Context, id string, disabled bool) error {
	_, err := s.pool.Exec(ctx, `UPDATE nodes SET is_disabled=$2, updated_at=NOW() WHERE uuid=$1`, id, disabled)
	return mapErr(err)
}

// NodeConnectedInfo is written when an agent completes its handshake.
type NodeConnectedInfo struct {
	AgentVersion  string
	XrayVersion   string
	Hostname      string
	OS            string
	Arch          string
	Kernel        string
	CPUCount      int
	CPUModel      string
	TotalRAMBytes int64
	ConfigHash    string
}

func (s *Store) MarkNodeConnected(ctx context.Context, id string, info NodeConnectedInfo) error {
	now := time.Now()
	_, err := s.pool.Exec(ctx, `UPDATE nodes SET health=$2, agent_version=$3, xray_version=$4, hostname=$5,
		os=$6, arch=$7, kernel=$8, cpu_count=$9, cpu_model=$10, total_ram_bytes=$11, config_hash=$12,
		last_connected_at=$13, last_status_at=$13, status_message='connected', updated_at=NOW()
		WHERE uuid=$1`,
		id, domain.NodeHealthConnecting, info.AgentVersion, info.XrayVersion, info.Hostname, info.OS,
		info.Arch, info.Kernel, info.CPUCount, info.CPUModel, info.TotalRAMBytes, info.ConfigHash, now)
	return mapErr(err)
}

func (s *Store) MarkNodeDisconnected(ctx context.Context, id, reason string) error {
	_, err := s.pool.Exec(ctx, `UPDATE nodes SET health=$2, xray_running=FALSE, online_users=0,
		cpu_usage=0, used_ram_bytes=0, load_avg_1=0, status_message=$3, last_status_at=NOW(), updated_at=NOW()
		WHERE uuid=$1`, id, domain.NodeHealthOffline, reason)
	return mapErr(err)
}

// MarkAllNodesDisconnected resets liveness state on panel start, since no agent
// stream survives a restart.
func (s *Store) MarkAllNodesDisconnected(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `UPDATE nodes SET health=$1, xray_running=FALSE, online_users=0,
		cpu_usage=0, used_ram_bytes=0, load_avg_1=0, status_message='panel restarted'
		WHERE health <> $1`, domain.NodeHealthOffline)
	return mapErr(err)
}

type NodeHeartbeat struct {
	Health        domain.NodeHealth
	XrayRunning   bool
	XrayStartedAt *time.Time
	XrayVersion   string
	ConfigHash    string
	CPUUsage      float64
	UsedRAMBytes  int64
	TotalRAMBytes int64
	LoadAvg1      float64
	OnlineUsers   int
	Message       string
}

func (s *Store) ApplyNodeHeartbeat(ctx context.Context, id string, hb NodeHeartbeat) error {
	_, err := s.pool.Exec(ctx, `UPDATE nodes SET health=$2, xray_running=$3, xray_started_at=$4,
		xray_version=COALESCE(NULLIF($5,''), xray_version), config_hash=$6, cpu_usage=$7, used_ram_bytes=$8,
		-- The cast is required: without it Postgres infers int4 from the
		-- comparison and rejects a realistic amount of RAM.
		total_ram_bytes=CASE WHEN $9::bigint > 0 THEN $9::bigint ELSE total_ram_bytes END, load_avg_1=$10,
		online_users=$11, status_message=$12, last_status_at=NOW(), updated_at=NOW()
		WHERE uuid=$1`,
		id, hb.Health, hb.XrayRunning, hb.XrayStartedAt, hb.XrayVersion, hb.ConfigHash, hb.CPUUsage,
		hb.UsedRAMBytes, hb.TotalRAMBytes, hb.LoadAvg1, hb.OnlineUsers, hb.Message)
	return mapErr(err)
}

func (s *Store) SetNodeConfigHash(ctx context.Context, id, hash string) error {
	_, err := s.pool.Exec(ctx, `UPDATE nodes SET config_hash=$2, updated_at=NOW() WHERE uuid=$1`, id, hash)
	return mapErr(err)
}

func (s *Store) SetNodeHealth(ctx context.Context, id string, health domain.NodeHealth, message string) error {
	_, err := s.pool.Exec(ctx, `UPDATE nodes SET health=$2, status_message=$3, last_status_at=NOW(), updated_at=NOW()
		WHERE uuid=$1`, id, health, message)
	return mapErr(err)
}

func (s *Store) AddNodeTraffic(ctx context.Context, id string, bytes int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE nodes SET traffic_used_bytes = traffic_used_bytes + $2 WHERE uuid=$1`, id, bytes)
	return mapErr(err)
}

func (s *Store) ResetNodeTraffic(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE nodes SET traffic_used_bytes = 0, last_traffic_reset_at = NOW() WHERE uuid=$1`, id)
	return mapErr(err)
}
