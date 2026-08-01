package postgres

import (
	"context"
	"time"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
)

// SpendSummary is the infrastructure cost picture shown on the dashboard.
type SpendSummary struct {
	Currency       string          `json:"currency"`
	MonthlyTotal   float64         `json:"monthlyTotal"`
	YearlyTotal    float64         `json:"yearlyTotal"`
	BilledNodes    int             `json:"billedNodes"`
	CostPerTB      float64         `json:"costPerTb"`
	TrafficMonthTB float64         `json:"trafficThisMonthTb"`
	ByProvider     []ProviderSpend `json:"byProvider"`
	Upcoming       []UpcomingBill  `json:"upcoming"`
	Overdue        int             `json:"overdue"`
}

type ProviderSpend struct {
	Provider string  `json:"provider"`
	Nodes    int     `json:"nodes"`
	Monthly  float64 `json:"monthly"`
}

type UpcomingBill struct {
	NodeID   string    `json:"nodeUuid"`
	NodeName string    `json:"nodeName"`
	Provider string    `json:"provider"`
	Amount   float64   `json:"amount"`
	Currency string    `json:"currency"`
	DueAt    time.Time `json:"dueAt"`
	DaysLeft int       `json:"daysLeft"`
}

// Spend aggregates node costs. Amounts are normalised to a monthly figure so
// nodes on different billing cycles can be compared and summed.
func (s *Store) Spend(ctx context.Context, defaultCurrency string) (*SpendSummary, error) {
	nodes, err := s.ListNodes(ctx)
	if err != nil {
		return nil, err
	}

	summary := &SpendSummary{
		Currency:   defaultCurrency,
		ByProvider: []ProviderSpend{},
		Upcoming:   []UpcomingBill{},
	}

	byProvider := map[string]*ProviderSpend{}
	now := time.Now()

	for i := range nodes {
		n := &nodes[i]
		if n.BillingCycle == "" || n.BillingCycle == domain.BillingNone || n.CostAmount <= 0 {
			continue
		}
		monthly := n.BillingCycle.MonthlyCost(n.CostAmount)
		summary.MonthlyTotal += monthly
		summary.BilledNodes++

		name := n.Provider
		if name == "" {
			name = "—"
		}
		entry := byProvider[name]
		if entry == nil {
			entry = &ProviderSpend{Provider: name}
			byProvider[name] = entry
		}
		entry.Nodes++
		entry.Monthly += monthly

		if n.NextPaymentAt != nil {
			days := int(n.NextPaymentAt.Sub(now).Hours() / 24)
			if days < 0 {
				summary.Overdue++
			}
			currency := n.CostCurrency
			if currency == "" {
				currency = defaultCurrency
			}
			// Anything already due or due within two months is worth showing.
			if days <= 62 {
				summary.Upcoming = append(summary.Upcoming, UpcomingBill{
					NodeID:   n.UUID,
					NodeName: n.Name,
					Provider: n.Provider,
					Amount:   n.CostAmount,
					Currency: currency,
					DueAt:    *n.NextPaymentAt,
					DaysLeft: days,
				})
			}
		}
	}

	summary.YearlyTotal = summary.MonthlyTotal * 12
	for _, v := range byProvider {
		summary.ByProvider = append(summary.ByProvider, *v)
	}
	// Biggest spend first, so the list answers "where is the money going".
	for i := 0; i < len(summary.ByProvider); i++ {
		for j := i + 1; j < len(summary.ByProvider); j++ {
			if summary.ByProvider[j].Monthly > summary.ByProvider[i].Monthly {
				summary.ByProvider[i], summary.ByProvider[j] = summary.ByProvider[j], summary.ByProvider[i]
			}
		}
	}
	// Soonest due first.
	for i := 0; i < len(summary.Upcoming); i++ {
		for j := i + 1; j < len(summary.Upcoming); j++ {
			if summary.Upcoming[j].DueAt.Before(summary.Upcoming[i].DueAt) {
				summary.Upcoming[i], summary.Upcoming[j] = summary.Upcoming[j], summary.Upcoming[i]
			}
		}
	}

	var monthBytes int64
	err = s.pool.QueryRow(ctx, `SELECT COALESCE(SUM(bytes), 0) FROM node_usage
		WHERE bucket >= date_trunc('month', NOW())`).Scan(&monthBytes)
	if err != nil {
		return nil, mapErr(err)
	}
	summary.TrafficMonthTB = float64(monthBytes) / (1 << 40)
	if summary.TrafficMonthTB > 0 {
		summary.CostPerTB = summary.MonthlyTotal / summary.TrafficMonthTB
	}
	return summary, nil
}

// RollDuePayments moves past-due nodes onto their next billing date and reports
// which ones rolled over, so the operator gets one notice per cycle.
func (s *Store) RollDuePayments(ctx context.Context) ([]domain.Node, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+nodeColumns+nodeFrom+`
		WHERE n.next_payment_at IS NOT NULL
		  AND n.next_payment_at < NOW()
		  AND n.billing_cycle <> 'NONE'`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	due := []domain.Node{}
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		due = append(due, *n)
	}
	if err := rows.Err(); err != nil {
		return nil, mapErr(err)
	}

	for i := range due {
		n := &due[i]
		next := n.BillingCycle.Advance(*n.NextPaymentAt)
		// A node left unpaid for several cycles catches up rather than firing
		// one notice per missed period.
		for next.Before(time.Now()) {
			next = n.BillingCycle.Advance(next)
		}
		if _, err := s.pool.Exec(ctx, `UPDATE nodes SET next_payment_at=$2 WHERE uuid=$1`, n.UUID, next); err != nil {
			return nil, mapErr(err)
		}
	}
	return due, nil
}

// ---------------------------------------------------------------- devices

// TouchDevice records a device against a subscription and reports whether the
// user is over their limit. A device already known never counts as new, so a
// returning client is not locked out by its own history.
func (s *Store) TouchDevice(ctx context.Context, userID, hwid, userAgent, platform string, limit int) (allowed bool, count int, err error) {
	if hwid == "" {
		return true, 0, nil
	}

	var known bool
	err = s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_devices WHERE user_uuid=$1 AND hwid=$2)`,
		userID, hwid).Scan(&known)
	if err != nil {
		return true, 0, mapErr(err)
	}

	if err = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM user_devices WHERE user_uuid=$1`, userID).Scan(&count); err != nil {
		return true, 0, mapErr(err)
	}

	if !known && limit > 0 && count >= limit {
		return false, count, nil
	}

	_, err = s.pool.Exec(ctx, `INSERT INTO user_devices (user_uuid, hwid, user_agent, platform)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (user_uuid, hwid) DO UPDATE
		SET last_seen = NOW(), user_agent = EXCLUDED.user_agent, platform = EXCLUDED.platform`,
		userID, hwid, userAgent, platform)
	if err != nil {
		return true, count, mapErr(err)
	}
	if !known {
		count++
	}
	return true, count, nil
}

func (s *Store) UserDevices(ctx context.Context, userID string) ([]domain.Device, error) {
	rows, err := s.pool.Query(ctx, `SELECT user_uuid, hwid, user_agent, platform, first_seen, last_seen
		FROM user_devices WHERE user_uuid=$1 ORDER BY last_seen DESC`, userID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	out := []domain.Device{}
	for rows.Next() {
		var d domain.Device
		if err := rows.Scan(&d.UserID, &d.HWID, &d.UserAgent, &d.Platform, &d.FirstSeen, &d.LastSeen); err != nil {
			return nil, mapErr(err)
		}
		out = append(out, d)
	}
	return out, mapErr(rows.Err())
}

func (s *Store) DeleteDevice(ctx context.Context, userID, hwid string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM user_devices WHERE user_uuid=$1 AND hwid=$2`, userID, hwid)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ResetDevices(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM user_devices WHERE user_uuid=$1`, userID)
	return mapErr(err)
}
