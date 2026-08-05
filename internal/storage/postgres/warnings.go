package postgres

import (
	"context"
	"time"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
)

// PendingWarning is one subscriber the operator should hear about before the
// thing happens, rather than after.
type PendingWarning struct {
	UserID   string `json:"userUuid"`
	Username string `json:"username"`
	// Days until expiry, for an expiry warning.
	DaysLeft int `json:"daysLeft,omitempty"`
	// Percent of quota used, for a quota warning.
	Percent int `json:"percent,omitempty"`
	// The figures behind the percent, so a message can name them.
	UsedBytes  int64 `json:"usedBytes,omitempty"`
	LimitBytes int64 `json:"limitBytes,omitempty"`
}

// ClaimExpiryWarnings returns the active users expiring within the window and
// marks them, so the caller can send one warning each.
//
// The claim and the read are one statement: the maintenance loop runs every
// minute, and a select-then-update would send the same warning again on the
// next tick if anything failed in between. Marking by the expiry timestamp
// rather than a boolean means a rescheduled expiry earns a fresh warning, which
// is what an operator extending a subscription expects.
func (s *Store) ClaimExpiryWarnings(ctx context.Context, within time.Duration) ([]PendingWarning, error) {
	if within <= 0 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		UPDATE users SET expiry_warned_for = expire_at
		WHERE status = $1
		  AND expire_at IS NOT NULL
		  AND expire_at > NOW()
		  AND expire_at <= NOW() + $2::interval
		  AND (expiry_warned_for IS NULL OR expiry_warned_for <> expire_at)
		RETURNING uuid, username,
		          GREATEST(0, CEIL(EXTRACT(EPOCH FROM (expire_at - NOW())) / 86400))::int`,
		domain.UserActive, within.String())
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []PendingWarning{}
	for rows.Next() {
		var w PendingWarning
		if err := rows.Scan(&w.UserID, &w.Username, &w.DaysLeft); err != nil {
			return nil, mapErr(err)
		}
		out = append(out, w)
	}
	return out, mapErr(rows.Err())
}

// ClaimQuotaWarnings returns the active users past the threshold and marks them
// at their current usage.
//
// Marking with the usage figure rather than a flag is what makes a reset clear
// the warning: after a monthly rollover used_traffic drops below the mark, the
// condition `used > quota_warned_at` is true again on the next crossing, and
// the subscriber is warned once per cycle rather than once ever.
func (s *Store) ClaimQuotaWarnings(ctx context.Context, percent int) ([]PendingWarning, error) {
	if percent <= 0 || percent > 100 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		UPDATE users SET quota_warned_at = used_traffic_bytes
		WHERE status = $1
		  AND traffic_limit_bytes > 0
		  AND used_traffic_bytes >= traffic_limit_bytes * $2 / 100
		  AND used_traffic_bytes < traffic_limit_bytes
		  AND used_traffic_bytes > quota_warned_at
		RETURNING uuid, username, used_traffic_bytes, traffic_limit_bytes,
		          (used_traffic_bytes * 100 / traffic_limit_bytes)::int`,
		domain.UserActive, percent)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []PendingWarning{}
	for rows.Next() {
		var w PendingWarning
		if err := rows.Scan(&w.UserID, &w.Username, &w.UsedBytes, &w.LimitBytes, &w.Percent); err != nil {
			return nil, mapErr(err)
		}
		out = append(out, w)
	}
	return out, mapErr(rows.Err())
}

// Attention is the short answer to "what needs me right now". Every figure is a
// count the dashboard can show without the operator opening four pages.
type Attention struct {
	NodesOffline    int              `json:"nodesOffline"`
	NodesDegraded   int              `json:"nodesDegraded"`
	NodesOverQuota  int              `json:"nodesOverQuota"`
	UsersExpiring   int              `json:"usersExpiring"`
	UsersNearQuota  int              `json:"usersNearQuota"`
	UsersLimited    int              `json:"usersLimited"`
	PaymentsDueSoon int              `json:"paymentsDueSoon"`
	Expiring        []PendingWarning `json:"expiring"`
	NearQuota       []PendingWarning `json:"nearQuota"`
}

// Attention counts without claiming anything — it is a read, and reading the
// dashboard must never consume a warning that has not been sent yet.
func (s *Store) Attention(ctx context.Context, expiryDays, quotaPercent int) (*Attention, error) {
	var a Attention
	if expiryDays <= 0 {
		expiryDays = 3
	}
	if quotaPercent <= 0 || quotaPercent > 100 {
		quotaPercent = 90
	}

	err := s.pool.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM nodes WHERE health = $1 AND NOT is_disabled),
		  (SELECT count(*) FROM nodes WHERE health = $2),
		  (SELECT count(*) FROM nodes WHERE health = $3),
		  (SELECT count(*) FROM users WHERE status = $4 AND expire_at IS NOT NULL
		     AND expire_at > NOW() AND expire_at <= NOW() + make_interval(days => $5)),
		  (SELECT count(*) FROM users WHERE status = $4 AND traffic_limit_bytes > 0
		     AND used_traffic_bytes >= traffic_limit_bytes * $6 / 100
		     AND used_traffic_bytes < traffic_limit_bytes),
		  (SELECT count(*) FROM users WHERE status = $7),
		  (SELECT count(*) FROM nodes WHERE next_payment_at IS NOT NULL
		     AND next_payment_at <= NOW() + interval '7 days')`,
		domain.NodeHealthOffline, domain.NodeHealthDegraded, domain.NodeHealthTrafficLimit,
		domain.UserActive, expiryDays, quotaPercent, domain.UserLimited,
	).Scan(&a.NodesOffline, &a.NodesDegraded, &a.NodesOverQuota,
		&a.UsersExpiring, &a.UsersNearQuota, &a.UsersLimited, &a.PaymentsDueSoon)
	if err != nil {
		return nil, mapErr(err)
	}

	// A handful of names each, so the card is actionable rather than a number
	// the operator still has to go looking for.
	a.Expiring, err = s.attentionUsers(ctx, `
		SELECT uuid, username,
		       GREATEST(0, CEIL(EXTRACT(EPOCH FROM (expire_at - NOW())) / 86400))::int, 0, 0, 0
		FROM users WHERE status = $1 AND expire_at IS NOT NULL
		  AND expire_at > NOW() AND expire_at <= NOW() + make_interval(days => $2)
		ORDER BY expire_at LIMIT 5`, domain.UserActive, expiryDays)
	if err != nil {
		return nil, err
	}
	a.NearQuota, err = s.attentionUsers(ctx, `
		SELECT uuid, username, 0,
		       (used_traffic_bytes * 100 / traffic_limit_bytes)::int,
		       used_traffic_bytes, traffic_limit_bytes
		FROM users WHERE status = $1 AND traffic_limit_bytes > 0
		  AND used_traffic_bytes >= traffic_limit_bytes * $2 / 100
		  AND used_traffic_bytes < traffic_limit_bytes
		ORDER BY used_traffic_bytes::float8 / traffic_limit_bytes DESC LIMIT 5`,
		domain.UserActive, quotaPercent)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) attentionUsers(ctx context.Context, q string, args ...any) ([]PendingWarning, error) {
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []PendingWarning{}
	for rows.Next() {
		var w PendingWarning
		if err := rows.Scan(&w.UserID, &w.Username, &w.DaysLeft, &w.Percent,
			&w.UsedBytes, &w.LimitBytes); err != nil {
			return nil, mapErr(err)
		}
		out = append(out, w)
	}
	return out, mapErr(rows.Err())
}
