// Package notify delivers panel events to the outside world.
//
// The panel already recorded everything worth knowing in its event log, but a
// log only helps someone who is already looking. A node going offline at 3am,
// a subscriber hitting their quota, a server payment coming due — those are
// worth pushing rather than waiting to be read.
package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
)

// Store is the slice of persistence the dispatcher needs.
type Store interface {
	EnabledChannels(ctx context.Context) ([]domain.NotificationChannel, error)
	RecordDelivery(ctx context.Context, d domain.NotificationDelivery) error
}

// Dispatcher fans events out to every channel subscribed to them.
//
// Delivery is deliberately decoupled from whatever produced the event: an admin
// clicking "disable user" must not wait on someone's webhook endpoint, and a
// dead endpoint must not turn into a failed API request. Events go onto a
// buffered queue and a worker drains it.
type Dispatcher struct {
	store  Store
	log    *slog.Logger
	client *http.Client
	queue  chan domain.Event
	done   chan struct{}

	// attempts and backoff are fields rather than constants so tests do not
	// have to sit through the real retry schedule.
	attempts int
	backoff  time.Duration
}

// New starts a dispatcher. Call Close to drain and stop it.
func New(store Store, log *slog.Logger) *Dispatcher {
	d := &Dispatcher{
		store: store,
		log:   log,
		// A per-request timeout, so one hanging endpoint cannot wedge the queue.
		client:   &http.Client{Timeout: 10 * time.Second},
		queue:    make(chan domain.Event, 256),
		done:     make(chan struct{}),
		attempts: 3,
		backoff:  time.Second,
	}
	go d.run()
	return d
}

// Publish queues an event. It never blocks: a panel that stalls because a
// notification queue is full is a worse outcome than a dropped notification,
// so a full queue drops and says so.
func (d *Dispatcher) Publish(e domain.Event) {
	if d == nil {
		return
	}
	select {
	case d.queue <- e:
	default:
		d.log.Warn("notification queue full, dropping event", "kind", e.Kind)
	}
}

func (d *Dispatcher) Close() {
	if d == nil {
		return
	}
	close(d.queue)
	<-d.done
}

// Test delivers one event to one channel and reports the outcome, bypassing
// both the queue and the channel's event subscription. An operator setting a
// webhook up needs the answer now, not the next time something happens to
// break — and they explicitly asked for this delivery, so the subscription
// filter would only get in the way.
//
// It records the attempt like any other, so a failed test is visible in the
// delivery log alongside real traffic.
func (d *Dispatcher) Test(ctx context.Context, c domain.NotificationChannel, e domain.Event) error {
	started := time.Now()

	var err error
	switch c.Kind {
	case domain.ChannelWebhook:
		err = d.postWebhook(ctx, c, e)
	case domain.ChannelTelegram:
		err = d.postTelegram(ctx, c, e)
	default:
		err = fmt.Errorf("unknown channel kind %q", c.Kind)
	}

	detail := "delivered"
	if err != nil {
		detail = err.Error()
	}
	_ = d.store.RecordDelivery(context.WithoutCancel(ctx), domain.NotificationDelivery{
		ChannelID:  c.UUID,
		EventKind:  e.Kind,
		OK:         err == nil,
		Detail:     truncate(detail, 500),
		Attempts:   1,
		DurationMS: int(time.Since(started).Milliseconds()),
	})
	return err
}

func (d *Dispatcher) run() {
	defer close(d.done)
	for e := range d.queue {
		d.deliver(e)
	}
}

func (d *Dispatcher) deliver(e domain.Event) {
	// A fresh context per event: the request that produced it is long gone.
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	channels, err := d.store.EnabledChannels(ctx)
	if err != nil {
		d.log.Warn("cannot load notification channels", "error", err)
		return
	}
	for _, c := range channels {
		if !c.Wants(e.Kind) {
			continue
		}
		d.send(ctx, c, e)
	}
}

func (d *Dispatcher) send(ctx context.Context, c domain.NotificationChannel, e domain.Event) {
	started := time.Now()
	var lastErr error
	attempts := 0

	for attempt := 0; attempt < d.attempts; attempt++ {
		attempts = attempt + 1
		if attempt > 0 {
			// Exponential backoff. An endpoint that is redeploying comes back
			// within seconds; one that is gone is not helped by hammering it.
			select {
			case <-time.After(d.backoff << (attempt - 1)):
			case <-ctx.Done():
				lastErr = ctx.Err()
				attempts = attempt
			}
			if ctx.Err() != nil {
				break
			}
		}

		switch c.Kind {
		case domain.ChannelWebhook:
			lastErr = d.postWebhook(ctx, c, e)
		case domain.ChannelTelegram:
			lastErr = d.postTelegram(ctx, c, e)
		default:
			lastErr = fmt.Errorf("unknown channel kind %q", c.Kind)
		}
		if lastErr == nil {
			break
		}
		if !retryable(lastErr) {
			break
		}
	}

	detail := "delivered"
	if lastErr != nil {
		detail = lastErr.Error()
	}
	_ = d.store.RecordDelivery(context.WithoutCancel(ctx), domain.NotificationDelivery{
		ChannelID:  c.UUID,
		EventKind:  e.Kind,
		OK:         lastErr == nil,
		Detail:     truncate(detail, 500),
		Attempts:   attempts,
		DurationMS: int(time.Since(started).Milliseconds()),
	})
	if lastErr != nil {
		d.log.Warn("notification failed", "channel", c.Name, "kind", e.Kind, "error", lastErr)
	}
}

// permanent marks a failure that will not be fixed by trying again — a
// malformed configuration, or a server that understood us and said no.
type permanent struct{ err error }

func (p permanent) Error() string { return p.err.Error() }

func retryable(err error) bool {
	var p permanent
	return !errorsAs(err, &p)
}

func errorsAs(err error, target *permanent) bool {
	if p, ok := err.(permanent); ok {
		*target = p
		return true
	}
	return false
}

// ---------------------------------------------------------------- webhook

type webhookConfig struct {
	URL    string `json:"url"`
	Secret string `json:"secret"`
}

// WebhookPayload is the JSON body posted to a webhook endpoint. It is a stable
// shape on purpose: receivers switch on `event` and read `subject`, and adding
// fields must not move the ones they already read.
type WebhookPayload struct {
	Event     domain.EventKind `json:"event"`
	Actor     string           `json:"actor,omitempty"`
	Subject   string           `json:"subject,omitempty"`
	Message   string           `json:"message,omitempty"`
	Meta      json.RawMessage  `json:"meta,omitempty"`
	Timestamp time.Time        `json:"timestamp"`
	Panel     string           `json:"panel"`
}

func (d *Dispatcher) postWebhook(ctx context.Context, c domain.NotificationChannel, e domain.Event) error {
	var cfg webhookConfig
	if err := json.Unmarshal(c.Config, &cfg); err != nil {
		return permanent{fmt.Errorf("bad webhook config: %w", err)}
	}
	if err := ValidateWebhookURL(cfg.URL); err != nil {
		return permanent{err}
	}

	body, err := json.Marshal(WebhookPayload{
		Event:     e.Kind,
		Actor:     e.Actor,
		Subject:   e.Subject,
		Message:   e.Message,
		Meta:      e.Meta,
		Timestamp: nonZero(e.CreatedAt),
		Panel:     "AmneziaX",
	})
	if err != nil {
		return permanent{err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return permanent{err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "AmneziaX")
	req.Header.Set("X-AmneziaX-Event", string(e.Kind))

	// A signature over the exact bytes sent, so a receiver can tell a genuine
	// delivery from anyone who learned the endpoint URL. Timestamped and signed
	// together so a captured body cannot be replayed indefinitely.
	if cfg.Secret != "" {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		mac := hmac.New(sha256.New, []byte(cfg.Secret))
		mac.Write([]byte(ts))
		mac.Write([]byte("."))
		mac.Write(body)
		req.Header.Set("X-AmneziaX-Timestamp", ts)
		req.Header.Set("X-AmneziaX-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	res, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4<<10))

	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}
	err = fmt.Errorf("endpoint returned %s", res.Status)
	// 4xx means the receiver understood and refused; repeating it changes
	// nothing. 5xx and 429 are worth another try.
	if res.StatusCode >= 400 && res.StatusCode < 500 && res.StatusCode != http.StatusTooManyRequests {
		return permanent{err}
	}
	return err
}

// ValidateWebhookURL rejects destinations a panel should never be talked into
// calling. The URL is operator-supplied, but an operator is not the only person
// who can end up owning an admin session, and a panel that will POST to any
// address is a request forgery primitive pointed at its own private network.
func ValidateWebhookURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("not a URL: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("webhook URL must be http or https")
	}
	if u.Host == "" {
		return fmt.Errorf("webhook URL has no host")
	}
	return nil
}

// ---------------------------------------------------------------- telegram

type telegramConfig struct {
	BotToken string `json:"botToken"`
	ChatID   string `json:"chatId"`
}

func (d *Dispatcher) postTelegram(ctx context.Context, c domain.NotificationChannel, e domain.Event) error {
	var cfg telegramConfig
	if err := json.Unmarshal(c.Config, &cfg); err != nil {
		return permanent{fmt.Errorf("bad telegram config: %w", err)}
	}
	if strings.TrimSpace(cfg.BotToken) == "" || strings.TrimSpace(cfg.ChatID) == "" {
		return permanent{fmt.Errorf("telegram needs a bot token and a chat id")}
	}

	payload, err := json.Marshal(map[string]any{
		"chat_id":    cfg.ChatID,
		"text":       TelegramText(e),
		"parse_mode": "HTML",
		// The panel's own links are not worth unfurling into a preview.
		"disable_web_page_preview": true,
	})
	if err != nil {
		return permanent{err}
	}

	endpoint := "https://api.telegram.org/bot" + url.PathEscape(cfg.BotToken) + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return permanent{err}
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4<<10))

	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}
	// Telegram explains itself in the body, and "chat not found" or "unauthorized"
	// is far more useful to an operator than the status code alone.
	detail := strings.TrimSpace(string(body))
	var parsed struct {
		Description string `json:"description"`
	}
	if json.Unmarshal(body, &parsed) == nil && parsed.Description != "" {
		detail = parsed.Description
	}
	err = fmt.Errorf("telegram: %s", detail)
	if res.StatusCode >= 400 && res.StatusCode < 500 && res.StatusCode != http.StatusTooManyRequests {
		return permanent{err}
	}
	return err
}

// TelegramText renders an event as the message a human reads on their phone.
func TelegramText(e domain.Event) string {
	var sb strings.Builder
	sb.WriteString(eventIcon(e.Kind))
	sb.WriteString(" <b>")
	sb.WriteString(escapeHTML(prettyKind(e.Kind)))
	sb.WriteString("</b>")
	if e.Subject != "" {
		sb.WriteString(" — <code>")
		sb.WriteString(escapeHTML(e.Subject))
		sb.WriteString("</code>")
	}
	if e.Message != "" {
		sb.WriteString("\n")
		sb.WriteString(escapeHTML(e.Message))
	}
	if e.Actor != "" {
		sb.WriteString("\n<i>by ")
		sb.WriteString(escapeHTML(e.Actor))
		sb.WriteString("</i>")
	}
	return sb.String()
}

func eventIcon(k domain.EventKind) string {
	switch k {
	case domain.EventNodeConnected:
		return "🟢"
	case domain.EventNodeDisconnected, domain.EventNodeError:
		return "🔴"
	case domain.EventUserLimited, domain.EventUserExpired, domain.EventDeviceBlocked:
		return "⚠️"
	case domain.EventNodePaymentDue:
		return "💳"
	case domain.EventAdminLoginFailed:
		return "🚫"
	case domain.EventUserCreated:
		return "➕"
	case domain.EventUserDeleted:
		return "➖"
	default:
		return "•"
	}
}

func prettyKind(k domain.EventKind) string {
	s := strings.ReplaceAll(strings.ToLower(string(k)), "_", " ")
	if s == "" {
		return "event"
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// escapeHTML covers the three characters Telegram's HTML parse mode treats as
// markup. A username containing one of them would otherwise break the message
// or, worse, inject formatting.
func escapeHTML(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func nonZero(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t
}
