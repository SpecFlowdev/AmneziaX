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
