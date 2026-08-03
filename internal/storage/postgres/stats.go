package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
)

// bucket truncates a timestamp to the hour, which is the resolution the panel
// keeps traffic history at.
func bucket(t time.Time) time.Time { return t.UTC().Truncate(time.Hour) }

func (s *Store) RecordNodeUsage(ctx context.Context, nodeID string, at time.Time, bytes int64) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO node_usage (node_uuid, bucket, bytes) VALUES ($1,$2,$3)
		ON CONFLICT (node_uuid, bucket) DO UPDATE SET bytes = node_usage.bytes + EXCLUDED.bytes`,
		nodeID, bucket(at), bytes)
	return mapErr(err)
}

func (s *Store) RecordUserUsage(ctx context.Context, userID, nodeID string, at time.Time, bytes int64) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO user_usage (user_uuid, node_uuid, bucket, bytes) VALUES ($1,$2,$3,$4)
		ON CONFLICT (user_uuid, node_uuid, bucket) DO UPDATE SET bytes = user_usage.bytes + EXCLUDED.bytes`,
		userID, nodeID, bucket(at), bytes)
	return mapErr(err)
}

// TrafficPoint is one row of a time series returned to the dashboard.
type TrafficPoint struct {
	At    time.Time `json:"at"`
	Bytes int64     `json:"bytes"`
}

// NodeSeries is per-node traffic over a window, grouped for stacked charts.
type NodeSeries struct {
	NodeID   string         `json:"nodeUuid"`
	NodeName string         `json:"nodeName"`
	Points   []TrafficPoint `json:"points"`
	Total    int64          `json:"totalBytes"`
}

func (s *Store) NodeTrafficSeries(ctx context.Context, since time.Time, interval string) ([]NodeSeries, error) {
	trunc := "hour"
	if interval == "day" {
		trunc = "day"
	}
	rows, err := s.pool.Query(ctx, `SELECT n.uuid, n.name, date_trunc('`+trunc+`', nu.bucket) AS ts, SUM(nu.bytes)
		FROM node_usage nu JOIN nodes n ON n.uuid = nu.node_uuid
		WHERE nu.bucket >= $1
		GROUP BY n.uuid, n.name, ts ORDER BY n.name, ts`, since)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	index := map[string]int{}
	out := []NodeSeries{}
	for rows.Next() {
		var id, name string
		var ts time.Time
		var b int64
		if err := rows.Scan(&id, &name, &ts, &b); err != nil {
			return nil, mapErr(err)
		}
		i, ok := index[id]
		if !ok {
			i = len(out)
			index[id] = i
			out = append(out, NodeSeries{NodeID: id, NodeName: name, Points: []TrafficPoint{}})
		}
		out[i].Points = append(out[i].Points, TrafficPoint{At: ts, Bytes: b})
		out[i].Total += b
	}
	return out, mapErr(rows.Err())
}

func (s *Store) TotalTrafficSeries(ctx context.Context, since time.Time, interval string) ([]TrafficPoint, error) {
	trunc := "hour"
	if interval == "day" {
		trunc = "day"
	}
	rows, err := s.pool.Query(ctx, `SELECT date_trunc('`+trunc+`', bucket) AS ts, SUM(bytes)
		FROM node_usage WHERE bucket >= $1 GROUP BY ts ORDER BY ts`, since)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []TrafficPoint{}
	for rows.Next() {
		var p TrafficPoint
		if err := rows.Scan(&p.At, &p.Bytes); err != nil {
			return nil, mapErr(err)
		}
		out = append(out, p)
	}
	return out, mapErr(rows.Err())
}

func (s *Store) UserTrafficSeries(ctx context.Context, userID string, since time.Time) ([]TrafficPoint, error) {
	rows, err := s.pool.Query(ctx, `SELECT bucket, SUM(bytes) FROM user_usage
		WHERE user_uuid=$1 AND bucket >= $2 GROUP BY bucket ORDER BY bucket`, userID, since)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []TrafficPoint{}
	for rows.Next() {
		var p TrafficPoint
		if err := rows.Scan(&p.At, &p.Bytes); err != nil {
			return nil, mapErr(err)
		}
		out = append(out, p)
	}
	return out, mapErr(rows.Err())
}

// TopUser is an entry of the "biggest consumers" dashboard widget.
type TopUser struct {
	UserID   string `json:"uuid"`
	Username string `json:"username"`
	Bytes    int64  `json:"bytes"`
}

func (s *Store) TopUsers(ctx context.Context, since time.Time, limit int) ([]TopUser, error) {
	rows, err := s.pool.Query(ctx, `SELECT u.uuid, u.username, SUM(uu.bytes) AS total
		FROM user_usage uu JOIN users u ON u.uuid = uu.user_uuid
		WHERE uu.bucket >= $1 GROUP BY u.uuid, u.username ORDER BY total DESC LIMIT $2`, since, limit)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []TopUser{}
	for rows.Next() {
		var t TopUser
		if err := rows.Scan(&t.UserID, &t.Username, &t.Bytes); err != nil {
			return nil, mapErr(err)
		}
		out = append(out, t)
	}
	return out, mapErr(rows.Err())
}

// Overview is the aggregate shown on the dashboard landing page.
type Overview struct {
	UsersTotal    int   `json:"usersTotal"`
	UsersActive   int   `json:"usersActive"`
	UsersDisabled int   `json:"usersDisabled"`
	UsersLimited  int   `json:"usersLimited"`
	UsersExpired  int   `json:"usersExpired"`
	UsersOnline   int   `json:"usersOnline"`
	NodesTotal    int   `json:"nodesTotal"`
	NodesOnline   int   `json:"nodesOnline"`
	NodesDisabled int   `json:"nodesDisabled"`
	HostsTotal    int   `json:"hostsTotal"`
	SquadsTotal   int   `json:"squadsTotal"`
	ProfilesTotal int   `json:"profilesTotal"`
	TrafficDay    int64 `json:"trafficLast24hBytes"`
	TrafficWeek   int64 `json:"trafficLast7dBytes"`
	TrafficMonth  int64 `json:"trafficLast30dBytes"`
	TrafficTotal  int64 `json:"trafficTotalBytes"`
}

func (s *Store) Overview(ctx context.Context) (*Overview, error) {
	var o Overview
	err := s.pool.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM users),
		(SELECT COUNT(*) FROM users WHERE status='ACTIVE'),
		(SELECT COUNT(*) FROM users WHERE status='DISABLED'),
		(SELECT COUNT(*) FROM users WHERE status='LIMITED'),
		(SELECT COUNT(*) FROM users WHERE status='EXPIRED'),
		(SELECT COUNT(*) FROM users WHERE online_at > NOW() - INTERVAL '3 minutes'),
		(SELECT COUNT(*) FROM nodes),
		(SELECT COUNT(*) FROM nodes WHERE health='ONLINE'),
		(SELECT COUNT(*) FROM nodes WHERE is_disabled),
		(SELECT COUNT(*) FROM hosts),
		(SELECT COUNT(*) FROM squads),
		(SELECT COUNT(*) FROM config_profiles),
		(SELECT COALESCE(SUM(bytes),0) FROM node_usage WHERE bucket >= NOW() - INTERVAL '24 hours'),
		(SELECT COALESCE(SUM(bytes),0) FROM node_usage WHERE bucket >= NOW() - INTERVAL '7 days'),
		(SELECT COALESCE(SUM(bytes),0) FROM node_usage WHERE bucket >= NOW() - INTERVAL '30 days'),
		(SELECT COALESCE(SUM(bytes),0) FROM node_usage)`).
		Scan(&o.UsersTotal, &o.UsersActive, &o.UsersDisabled, &o.UsersLimited, &o.UsersExpired, &o.UsersOnline,
			&o.NodesTotal, &o.NodesOnline, &o.NodesDisabled, &o.HostsTotal, &o.SquadsTotal, &o.ProfilesTotal,
			&o.TrafficDay, &o.TrafficWeek, &o.TrafficMonth, &o.TrafficTotal)
	if err != nil {
		return nil, mapErr(err)
	}
	return &o, nil
}

// PruneUsage drops history older than the retention window.
func (s *Store) PruneUsage(ctx context.Context, retention time.Duration) error {
	cutoff := time.Now().Add(-retention)
	if _, err := s.pool.Exec(ctx, `DELETE FROM node_usage WHERE bucket < $1`, cutoff); err != nil {
		return mapErr(err)
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM user_usage WHERE bucket < $1`, cutoff); err != nil {
		return mapErr(err)
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM events WHERE created_at < $1`, cutoff)
	return mapErr(err)
}

func (s *Store) LogEvent(ctx context.Context, kind domain.EventKind, actor, subject, message string, meta any) {
	var raw []byte
	if meta != nil {
		raw, _ = json.Marshal(meta)
	}
	_, _ = s.pool.Exec(ctx, `INSERT INTO events (kind, actor, subject, message, meta) VALUES ($1,$2,$3,$4,$5)`,
		kind, actor, subject, message, raw)

	if s.onEvent != nil {
		s.onEvent(domain.Event{
			Kind: kind, Actor: actor, Subject: subject, Message: message,
			Meta: raw, CreatedAt: time.Now().UTC(),
		})
	}
}

func (s *Store) ListEvents(ctx context.Context, limit int, kind string) ([]domain.Event, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `SELECT id, kind, actor, subject, message, meta, created_at FROM events`
	args := []any{}
	if kind != "" {
		query += ` WHERE kind = $1`
		args = append(args, kind)
	}
	args = append(args, limit)
	if kind != "" {
		query += ` ORDER BY created_at DESC LIMIT $2`
	} else {
		query += ` ORDER BY created_at DESC LIMIT $1`
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []domain.Event{}
	for rows.Next() {
		var e domain.Event
		if err := rows.Scan(&e.ID, &e.Kind, &e.Actor, &e.Subject, &e.Message, &e.Meta, &e.CreatedAt); err != nil {
			return nil, mapErr(err)
		}
		out = append(out, e)
	}
	return out, mapErr(rows.Err())
}
