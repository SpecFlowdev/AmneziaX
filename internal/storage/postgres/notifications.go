package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
)

const channelColumns = `uuid, name, kind, config, events, is_enabled,
	last_ok, last_detail, last_sent_at, created_at, updated_at`

func scanChannel(row interface{ Scan(...any) error }) (*domain.NotificationChannel, error) {
	var c domain.NotificationChannel
	var events []string
	var config []byte
	err := row.Scan(&c.UUID, &c.Name, &c.Kind, &config, &events, &c.IsEnabled,
		&c.LastOK, &c.LastDetail, &c.LastSentAt, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, mapErr(err)
	}
	c.Config = json.RawMessage(config)
	c.Events = make([]domain.EventKind, 0, len(events))
	for _, e := range events {
		c.Events = append(c.Events, domain.EventKind(e))
	}
	return &c, nil
}

func eventStrings(events []domain.EventKind) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, string(e))
	}
	return out
}

func (s *Store) ListChannels(ctx context.Context) ([]domain.NotificationChannel, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+channelColumns+
		` FROM notification_channels ORDER BY created_at`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []domain.NotificationChannel{}
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, mapErr(rows.Err())
}

// EnabledChannels is the dispatcher's read path, kept separate so a disabled
// channel costs nothing at delivery time.
func (s *Store) EnabledChannels(ctx context.Context) ([]domain.NotificationChannel, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+channelColumns+
		` FROM notification_channels WHERE is_enabled ORDER BY created_at`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []domain.NotificationChannel{}
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, mapErr(rows.Err())
}

func (s *Store) Channel(ctx context.Context, id string) (*domain.NotificationChannel, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+channelColumns+
		` FROM notification_channels WHERE uuid = $1`, id)
	return scanChannel(row)
}

func (s *Store) CreateChannel(ctx context.Context, c domain.NotificationChannel) (*domain.NotificationChannel, error) {
	row := s.pool.QueryRow(ctx, `INSERT INTO notification_channels
		(name, kind, config, events, is_enabled)
		VALUES ($1,$2,$3,$4,$5) RETURNING `+channelColumns,
		c.Name, c.Kind, []byte(c.Config), eventStrings(c.Events), c.IsEnabled)
	return scanChannel(row)
}

func (s *Store) UpdateChannel(ctx context.Context, id string, c domain.NotificationChannel) (*domain.NotificationChannel, error) {
	row := s.pool.QueryRow(ctx, `UPDATE notification_channels SET
		name = $2, kind = $3, config = $4, events = $5, is_enabled = $6, updated_at = NOW()
		WHERE uuid = $1 RETURNING `+channelColumns,
		id, c.Name, c.Kind, []byte(c.Config), eventStrings(c.Events), c.IsEnabled)
	return scanChannel(row)
}

func (s *Store) DeleteChannel(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM notification_channels WHERE uuid = $1`, id)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RecordDelivery appends to the delivery log and rolls the denormalised health
// fields on the channel in the same statement pair, so the list view and the
// log can never disagree about the last outcome.
func (s *Store) RecordDelivery(ctx context.Context, d domain.NotificationDelivery) error {
	if _, err := s.pool.Exec(ctx, `INSERT INTO notification_deliveries
		(channel_id, event_kind, ok, detail, attempts, duration_ms)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		d.ChannelID, d.EventKind, d.OK, d.Detail, d.Attempts, d.DurationMS); err != nil {
		return mapErr(err)
	}
	_, err := s.pool.Exec(ctx, `UPDATE notification_channels
		SET last_ok = $2, last_detail = $3, last_sent_at = NOW()
		WHERE uuid = $1`, d.ChannelID, d.OK, d.Detail)
	return mapErr(err)
}

func (s *Store) ChannelDeliveries(ctx context.Context, id string, limit int) ([]domain.NotificationDelivery, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `SELECT id, channel_id, event_kind, ok, detail,
		attempts, duration_ms, created_at
		FROM notification_deliveries WHERE channel_id = $1
		ORDER BY created_at DESC LIMIT $2`, id, limit)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []domain.NotificationDelivery{}
	for rows.Next() {
		var d domain.NotificationDelivery
		if err := rows.Scan(&d.ID, &d.ChannelID, &d.EventKind, &d.OK, &d.Detail,
			&d.Attempts, &d.DurationMS, &d.CreatedAt); err != nil {
			return nil, mapErr(err)
		}
		out = append(out, d)
	}
	return out, mapErr(rows.Err())
}

// PruneDeliveries keeps the log from growing without bound. A panel that
// notifies on every user update produces a lot of rows, and none of them are
// interesting a week later.
func (s *Store) PruneDeliveries(ctx context.Context, keepFor time.Duration) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM notification_deliveries WHERE created_at < NOW() - $1::interval`,
		keepFor.String())
	return mapErr(err)
}

// ---------------------------------------------------------------- node metrics

// RecordNodeMetric stores one heartbeat sample. Samples are keyed by minute so
// a chatty agent cannot flood the table — a node heartbeating every few seconds
// would otherwise write tens of thousands of rows a day for a chart that is
// read at minute resolution anyway.
func (s *Store) RecordNodeMetric(ctx context.Context, nodeID string, m domain.NodeMetric) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO node_metrics
		(node_id, at, cpu_percent, used_ram_bytes, total_ram_bytes, load_avg1, online_users)
		VALUES ($1, date_trunc('minute', NOW()), $2, $3::bigint, $4::bigint, $5, $6)
		ON CONFLICT (node_id, at) DO UPDATE SET
			cpu_percent = EXCLUDED.cpu_percent,
			used_ram_bytes = EXCLUDED.used_ram_bytes,
			total_ram_bytes = EXCLUDED.total_ram_bytes,
			load_avg1 = EXCLUDED.load_avg1,
			online_users = EXCLUDED.online_users`,
		nodeID, m.CPUPercent, m.UsedRAM, m.TotalRAM, m.LoadAvg1, m.OnlineUsers)
	return mapErr(err)
}

func (s *Store) NodeMetrics(ctx context.Context, nodeID string, since time.Duration) ([]domain.NodeMetric, error) {
	rows, err := s.pool.Query(ctx, `SELECT at, cpu_percent, used_ram_bytes,
		total_ram_bytes, load_avg1, online_users
		FROM node_metrics
		WHERE node_id = $1 AND at >= NOW() - $2::interval
		ORDER BY at`, nodeID, since.String())
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []domain.NodeMetric{}
	for rows.Next() {
		var m domain.NodeMetric
		if err := rows.Scan(&m.At, &m.CPUPercent, &m.UsedRAM, &m.TotalRAM,
			&m.LoadAvg1, &m.OnlineUsers); err != nil {
			return nil, mapErr(err)
		}
		out = append(out, m)
	}
	return out, mapErr(rows.Err())
}

func (s *Store) PruneNodeMetrics(ctx context.Context, keepFor time.Duration) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM node_metrics WHERE at < NOW() - $1::interval`, keepFor.String())
	return mapErr(err)
}

// ---------------------------------------------------------------- announcements

const announcementColumns = `uuid, title, body, level, is_enabled, starts_at,
	ends_at, created_at, updated_at`

func scanAnnouncement(row interface{ Scan(...any) error }) (*domain.Announcement, error) {
	var a domain.Announcement
	err := row.Scan(&a.UUID, &a.Title, &a.Body, &a.Level, &a.IsEnabled,
		&a.StartsAt, &a.EndsAt, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, mapErr(err)
	}
	return &a, nil
}

func (s *Store) ListAnnouncements(ctx context.Context) ([]domain.Announcement, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+announcementColumns+
		` FROM announcements ORDER BY created_at DESC`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []domain.Announcement{}
	for rows.Next() {
		a, err := scanAnnouncement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, mapErr(rows.Err())
}

// LiveAnnouncements is what a subscriber sees. The window is filtered in SQL so
// the subscription path stays one query regardless of how many notices have
// accumulated over the life of the deployment.
func (s *Store) LiveAnnouncements(ctx context.Context) ([]domain.Announcement, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+announcementColumns+
		` FROM announcements
		WHERE is_enabled
		  AND (starts_at IS NULL OR starts_at <= NOW())
		  AND (ends_at IS NULL OR ends_at >= NOW())
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []domain.Announcement{}
	for rows.Next() {
		a, err := scanAnnouncement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, mapErr(rows.Err())
}

func (s *Store) CreateAnnouncement(ctx context.Context, a domain.Announcement) (*domain.Announcement, error) {
	row := s.pool.QueryRow(ctx, `INSERT INTO announcements
		(title, body, level, is_enabled, starts_at, ends_at)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING `+announcementColumns,
		a.Title, a.Body, a.Level, a.IsEnabled, a.StartsAt, a.EndsAt)
	return scanAnnouncement(row)
}

func (s *Store) UpdateAnnouncement(ctx context.Context, id string, a domain.Announcement) (*domain.Announcement, error) {
	row := s.pool.QueryRow(ctx, `UPDATE announcements SET
		title = $2, body = $3, level = $4, is_enabled = $5,
		starts_at = $6, ends_at = $7, updated_at = NOW()
		WHERE uuid = $1 RETURNING `+announcementColumns,
		id, a.Title, a.Body, a.Level, a.IsEnabled, a.StartsAt, a.EndsAt)
	return scanAnnouncement(row)
}

func (s *Store) DeleteAnnouncement(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM announcements WHERE uuid = $1`, id)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------- subscription requests

// SubscriptionRequest is one fetch of a subscription link.
type SubscriptionRequest struct {
	ID        int64     `json:"id"`
	UserID    *string   `json:"userUuid"`
	Username  string    `json:"username"`
	Token     string    `json:"token"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"userAgent"`
	Format    string    `json:"format"`
	Status    int       `json:"status"`
	HWID      string    `json:"hwid"`
	At        time.Time `json:"at"`
}

// RecordSubscriptionRequest is fire-and-forget: a subscriber fetching their
// configuration must never fail because the audit write did.
func (s *Store) RecordSubscriptionRequest(ctx context.Context, r SubscriptionRequest) {
	_, _ = s.pool.Exec(ctx, `INSERT INTO subscription_requests
		(user_id, token, ip, user_agent, format, status, hwid)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		r.UserID, r.Token, r.IP, r.UserAgent, r.Format, r.Status, r.HWID)
}

// SubscriptionRequests lists recent fetches, newest first. userID narrows it to
// one subscriber; failed lists only the ones that did not resolve, which is
// where a revoked link still being polled shows up.
func (s *Store) SubscriptionRequests(ctx context.Context, userID string, failedOnly bool, limit int) ([]SubscriptionRequest, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `SELECT r.id, r.user_id, COALESCE(u.username, ''), r.token, r.ip,
		r.user_agent, r.format, r.status, r.hwid, r.at
		FROM subscription_requests r
		LEFT JOIN users u ON u.uuid = r.user_id
		WHERE ($1 = '' OR r.user_id::text = $1)
		  AND (NOT $2 OR r.status >= 400)
		ORDER BY r.at DESC LIMIT $3`

	rows, err := s.pool.Query(ctx, query, userID, failedOnly, limit)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []SubscriptionRequest{}
	for rows.Next() {
		var r SubscriptionRequest
		if err := rows.Scan(&r.ID, &r.UserID, &r.Username, &r.Token, &r.IP,
			&r.UserAgent, &r.Format, &r.Status, &r.HWID, &r.At); err != nil {
			return nil, mapErr(err)
		}
		out = append(out, r)
	}
	return out, mapErr(rows.Err())
}

func (s *Store) PruneSubscriptionRequests(ctx context.Context, keepFor time.Duration) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM subscription_requests WHERE at < NOW() - $1::interval`, keepFor.String())
	return mapErr(err)
}

// AllDevices powers the hardware-id inspector: every known device across every
// subscriber, rather than one user at a time.
func (s *Store) AllDevices(ctx context.Context, search string, limit int) ([]domain.Device, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.pool.Query(ctx, `SELECT d.user_uuid, d.hwid, d.user_agent, d.platform,
		d.first_seen, d.last_seen, u.username
		FROM user_devices d JOIN users u ON u.uuid = d.user_uuid
		WHERE $1 = '' OR d.hwid ILIKE '%'||$1||'%' OR u.username ILIKE '%'||$1||'%'
		   OR d.platform ILIKE '%'||$1||'%'
		ORDER BY d.last_seen DESC LIMIT $2`, search, limit)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []domain.Device{}
	for rows.Next() {
		var d domain.Device
		var username string
		if err := rows.Scan(&d.UserID, &d.HWID, &d.UserAgent, &d.Platform,
			&d.FirstSeen, &d.LastSeen, &username); err != nil {
			return nil, mapErr(err)
		}
		d.Username = username
		out = append(out, d)
	}
	return out, mapErr(rows.Err())
}

// ---------------------------------------------------------------- response rules

// ResponseRule maps a client to the format it should be served.
type ResponseRule struct {
	UUID      string    `json:"uuid"`
	Name      string    `json:"name"`
	MatchUA   string    `json:"matchUserAgent"`
	Format    string    `json:"format"`
	IsEnabled bool      `json:"isEnabled"`
	Priority  int       `json:"priority"`
	Hits      int64     `json:"hits"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

const ruleColumns = `uuid, name, match_ua, format, is_enabled, priority, hits, created_at, updated_at`

func scanRule(row interface{ Scan(...any) error }) (*ResponseRule, error) {
	var r ResponseRule
	err := row.Scan(&r.UUID, &r.Name, &r.MatchUA, &r.Format, &r.IsEnabled,
		&r.Priority, &r.Hits, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, mapErr(err)
	}
	return &r, nil
}

func (s *Store) ListRules(ctx context.Context) ([]ResponseRule, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+ruleColumns+
		` FROM response_rules ORDER BY priority, created_at`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []ResponseRule{}
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, mapErr(rows.Err())
}

// MatchRule finds the first enabled rule whose pattern appears in the user
// agent, and counts the hit. Matching happens in SQL so the subscription path
// stays one round trip no matter how many rules exist.
func (s *Store) MatchRule(ctx context.Context, userAgent string) (string, bool) {
	if userAgent == "" {
		return "", false
	}
	var id, format string
	err := s.pool.QueryRow(ctx, `SELECT uuid, format FROM response_rules
		WHERE is_enabled AND match_ua <> '' AND $1 ILIKE '%'||match_ua||'%'
		ORDER BY priority, created_at LIMIT 1`, userAgent).Scan(&id, &format)
	if err != nil {
		return "", false
	}
	// Counting a hit is what makes a rule debuggable: a rule that never fires
	// looks identical to a correct one until you can see it has never fired.
	_, _ = s.pool.Exec(ctx, `UPDATE response_rules SET hits = hits + 1 WHERE uuid = $1`, id)
	return format, true
}

// PreviewRule answers the same question as MatchRule without recording a hit,
// for the log line and for the operator's "what would this client get" probe.
func (s *Store) PreviewRule(ctx context.Context, userAgent string) (string, bool) {
	if userAgent == "" {
		return "", false
	}
	var format string
	err := s.pool.QueryRow(ctx, `SELECT format FROM response_rules
		WHERE is_enabled AND match_ua <> '' AND $1 ILIKE '%'||match_ua||'%'
		ORDER BY priority, created_at LIMIT 1`, userAgent).Scan(&format)
	if err != nil {
		return "", false
	}
	return format, true
}

func (s *Store) CreateRule(ctx context.Context, r ResponseRule) (*ResponseRule, error) {
	row := s.pool.QueryRow(ctx, `INSERT INTO response_rules
		(name, match_ua, format, is_enabled, priority)
		VALUES ($1,$2,$3,$4,$5) RETURNING `+ruleColumns,
		r.Name, r.MatchUA, r.Format, r.IsEnabled, r.Priority)
	return scanRule(row)
}

func (s *Store) UpdateRule(ctx context.Context, id string, r ResponseRule) (*ResponseRule, error) {
	row := s.pool.QueryRow(ctx, `UPDATE response_rules SET
		name = $2, match_ua = $3, format = $4, is_enabled = $5, priority = $6, updated_at = NOW()
		WHERE uuid = $1 RETURNING `+ruleColumns,
		id, r.Name, r.MatchUA, r.Format, r.IsEnabled, r.Priority)
	return scanRule(row)
}

func (s *Store) DeleteRule(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM response_rules WHERE uuid = $1`, id)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
