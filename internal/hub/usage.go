package hub

import (
	"context"
	"strings"
	"time"

	nodev1 "github.com/SpecFlowdev/AmneziaX/gen/go/node/v1"
	"github.com/SpecFlowdev/AmneziaX/internal/domain"
)

// handleUsage charges a usage report to the users and the node that produced it.
// Agents reset xray counters when they read them, so every number here is a
// delta since the previous report.
func (h *Hub) handleUsage(ctx context.Context, nodeID string, report *nodev1.UsageReport) {
	at := time.Now()
	if ts := report.GetCollectedAtUnix(); ts > 0 {
		at = time.Unix(ts, 0)
	}

	node, err := h.store.Node(ctx, nodeID)
	if err != nil {
		h.log.Error("usage: load node", "node", nodeID, "error", err)
		return
	}
	multiplier := node.ConsumptionMultip
	if multiplier <= 0 {
		multiplier = 1
	}

	var nodeTotal int64
	limitedUsers := make([]string, 0)

	for _, u := range report.GetUsers() {
		userID, ok := userIDFromEmail(u.GetEmail())
		if !ok {
			continue
		}
		raw := int64(u.GetUplinkBytes() + u.GetDownlinkBytes())
		if raw <= 0 {
			if u.GetOnline() {
				h.store.MarkUserOnline(ctx, userID)
			}
			continue
		}
		charged := int64(float64(raw) * multiplier)

		used, limit, statusBefore, err := h.store.AddUserTraffic(ctx, userID, charged)
		if err != nil {
			h.log.Warn("usage: charge user", "user", userID, "error", err)
			continue
		}
		if err := h.store.RecordUserUsage(ctx, userID, nodeID, at, charged); err != nil {
			h.log.Warn("usage: record user history", "user", userID, "error", err)
		}
		if limit > 0 && used >= limit && statusBefore == domain.UserActive {
			if err := h.store.SetUserStatus(ctx, userID, domain.UserLimited); err == nil {
				limitedUsers = append(limitedUsers, userID)
			}
		}
	}

	for _, in := range report.GetInbounds() {
		nodeTotal += int64(float64(in.GetUplinkBytes()+in.GetDownlinkBytes()) * multiplier)
	}
	if nodeTotal > 0 {
		if err := h.store.AddNodeTraffic(ctx, nodeID, nodeTotal); err != nil {
			h.log.Warn("usage: charge node", "node", nodeID, "error", err)
		}
		if err := h.store.RecordNodeUsage(ctx, nodeID, at, nodeTotal); err != nil {
			h.log.Warn("usage: record node history", "node", nodeID, "error", err)
		}
		if node.TrafficLimitBytes > 0 && node.TrafficUsedBytes+nodeTotal >= node.TrafficLimitBytes {
			h.log.Warn("node reached its traffic limit", "node", node.Name)
			h.store.LogEvent(ctx, domain.EventNodeError, "system", node.Name, "traffic limit reached", nil)
			_ = h.store.SetNodeHealth(ctx, nodeID, domain.NodeHealthTrafficLimit, "traffic limit reached")
			_ = h.stopNode(ctx, node)
		}
	}

	// A user who just hit their quota must be removed from the running config,
	// otherwise they keep the tunnel until the next unrelated edit.
	for _, userID := range limitedUsers {
		h.store.LogEvent(ctx, domain.EventUserLimited, "system", userID, "traffic limit reached", nil)
		h.RequestSyncForUser(ctx, userID)
	}
}

// userIDFromEmail recovers the user's uuid from the xray stats email, which the
// panel formats as "<uuid>.<username>".
func userIDFromEmail(email string) (string, bool) {
	idx := strings.Index(email, ".")
	if idx <= 0 {
		return "", false
	}
	id := email[:idx]
	if len(id) != 36 {
		return "", false
	}
	return id, true
}

// RunMaintenance applies scheduled housekeeping: expiring users, rolling traffic
// counters over and pruning history.
func (h *Hub) RunMaintenance(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		expired, err := h.store.ExpireUsers(ctx)
		if err != nil {
			h.log.Error("maintenance: expire users", "error", err)
		} else if expired > 0 {
			h.log.Info("users expired", "count", expired)
			h.store.LogEvent(ctx, domain.EventUserExpired, "system", "", "expired users removed from nodes",
				map[string]any{"count": expired})
			h.RequestSyncAll(ctx)
		}

		reset, err := h.store.ResetDueUserTraffic(ctx)
		if err != nil {
			h.log.Error("maintenance: reset traffic", "error", err)
		} else if reset > 0 {
			h.log.Info("user traffic counters reset", "count", reset)
			h.RequestSyncAll(ctx)
		}

		if err := h.store.PruneUsage(ctx, h.cfg.UsageRetention); err != nil {
			h.log.Error("maintenance: prune usage", "error", err)
		}

		h.warnBeforeCutoff(ctx)

		// Heartbeat samples and delivery receipts both accumulate a row per
		// minute per node and per notification, and neither is interesting once
		// the window the UI draws has moved past it.
		if err := h.store.PruneNodeMetrics(ctx, 30*24*time.Hour); err != nil {
			h.log.Error("maintenance: prune node metrics", "error", err)
		}
		if err := h.store.PruneDeliveries(ctx, 14*24*time.Hour); err != nil {
			h.log.Error("maintenance: prune deliveries", "error", err)
		}
		if err := h.store.PruneSubscriptionRequests(ctx, 30*24*time.Hour); err != nil {
			h.log.Error("maintenance: prune subscription requests", "error", err)
		}

		// Nodes whose rental period has elapsed roll onto the next date and
		// leave one event behind, so the operator sees what has to be paid.
		due, err := h.store.RollDuePayments(ctx)
		if err != nil {
			h.log.Error("maintenance: node billing", "error", err)
		}
		for i := range due {
			n := &due[i]
			h.log.Info("node payment due", "node", n.Name, "provider", n.Provider,
				"amount", n.CostAmount, "currency", n.CostCurrency)
			h.store.LogEvent(ctx, domain.EventNodePaymentDue, "system", n.Name,
				"rental period ended — payment due",
				map[string]any{
					"provider": n.Provider,
					"amount":   n.CostAmount,
					"currency": n.CostCurrency,
					"cycle":    n.BillingCycle,
				})
		}
	}
}

// warnBeforeCutoff tells the operator about subscribers who are about to lose
// service, while there is still time to do something about it. Both claims mark
// the user as they read, so each warning is emitted once rather than once a
// minute until the deadline arrives.
func (h *Hub) warnBeforeCutoff(ctx context.Context) {
	settings, err := h.store.Settings(ctx)
	if err != nil {
		h.log.Warn("maintenance: read settings for warnings", "error", err)
		return
	}

	expiring, err := h.store.ClaimExpiryWarnings(ctx,
		time.Duration(settings.WarnExpiryDays)*24*time.Hour)
	if err != nil {
		h.log.Error("maintenance: expiry warnings", "error", err)
	}
	for i := range expiring {
		w := &expiring[i]
		h.store.LogEvent(ctx, domain.EventUserExpiringSoon, "system", w.Username,
			"subscription expires soon",
			map[string]any{"daysLeft": w.DaysLeft})
	}

	nearQuota, err := h.store.ClaimQuotaWarnings(ctx, settings.WarnQuotaPercent)
	if err != nil {
		h.log.Error("maintenance: quota warnings", "error", err)
	}
	for i := range nearQuota {
		w := &nearQuota[i]
		h.store.LogEvent(ctx, domain.EventUserQuotaWarning, "system", w.Username,
			"traffic quota nearly used up",
			map[string]any{
				"percent":    w.Percent,
				"usedBytes":  w.UsedBytes,
				"limitBytes": w.LimitBytes,
			})
	}

	if n := len(expiring) + len(nearQuota); n > 0 {
		h.log.Info("subscribers warned before cutoff",
			"expiring", len(expiring), "nearQuota", len(nearQuota))
	}
}
