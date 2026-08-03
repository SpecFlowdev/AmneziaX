package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
	"github.com/SpecFlowdev/AmneziaX/internal/notify"
	"github.com/SpecFlowdev/AmneziaX/internal/storage/postgres"
	"github.com/go-chi/chi/v5"
)

// channelRequest is the write shape. Config is transport-specific and validated
// per kind rather than by struct tags, because a webhook and a bot share no
// fields.
type channelRequest struct {
	Name      string             `json:"name"`
	Kind      domain.ChannelKind `json:"kind"`
	Config    json.RawMessage    `json:"config"`
	Events    []domain.EventKind `json:"events"`
	IsEnabled *bool              `json:"isEnabled"`
}

// channelView hides the secrets. A signing secret and a bot token are
// write-only: an admin who can read them can forge deliveries or take over the
// bot, and there is no workflow that needs them back out of the panel.
type channelView struct {
	domain.NotificationChannel
	Config     map[string]any `json:"config"`
	HasSecret  bool           `json:"hasSecret"`
	EventCount int            `json:"eventCount"`
}

func viewChannel(c domain.NotificationChannel) channelView {
	raw := map[string]any{}
	_ = json.Unmarshal(c.Config, &raw)

	redacted := map[string]any{}
	hasSecret := false
	for k, v := range raw {
		if isSecretField(k) {
			if s, _ := v.(string); s != "" {
				hasSecret = true
			}
			continue
		}
		redacted[k] = v
	}

	c.Config = nil
	return channelView{
		NotificationChannel: c,
		Config:              redacted,
		HasSecret:           hasSecret,
		EventCount:          len(c.Events),
	}
}

func isSecretField(name string) bool {
	switch strings.ToLower(name) {
	case "secret", "bottoken", "token", "password":
		return true
	}
	return false
}

func (a *API) listChannels(w http.ResponseWriter, r *http.Request) {
	channels, err := a.store.ListChannels(r.Context())
	if err != nil {
		a.storeErr(w, err)
		return
	}
	out := make([]channelView, 0, len(channels))
	for _, c := range channels {
		out = append(out, viewChannel(c))
	}
	writeJSON(w, http.StatusOK, out)
}

// normaliseChannel validates a write and returns the record to store. existing
// is the current row on update, so an edit that leaves a secret field blank
// keeps the stored one instead of silently clearing it — the UI never had the
// value to send back.
func normaliseChannel(req channelRequest, existing *domain.NotificationChannel) (domain.NotificationChannel, error) {
	var c domain.NotificationChannel

	c.Name = strings.TrimSpace(req.Name)
	if c.Name == "" {
		return c, errors.New("the channel needs a name")
	}
	if len(c.Name) > 64 {
		return c, errors.New("the name is too long")
	}

	c.Kind = domain.ChannelKind(strings.ToUpper(strings.TrimSpace(string(req.Kind))))
	if !c.Kind.Valid() {
		return c, errors.New("unknown channel kind")
	}

	for _, e := range req.Events {
		if !e.Valid() {
			return c, errors.New("unknown event " + string(e))
		}
		c.Events = append(c.Events, e)
	}

	c.IsEnabled = true
	if req.IsEnabled != nil {
		c.IsEnabled = *req.IsEnabled
	}

	incoming := map[string]any{}
	if len(req.Config) > 0 {
		if err := json.Unmarshal(req.Config, &incoming); err != nil {
			return c, errors.New("the configuration is not valid JSON")
		}
	}
	if existing != nil {
		current := map[string]any{}
		_ = json.Unmarshal(existing.Config, &current)
		for k, v := range current {
			if !isSecretField(k) {
				continue
			}
			if s, ok := incoming[k].(string); !ok || strings.TrimSpace(s) == "" {
				incoming[k] = v
			}
		}
	}

	switch c.Kind {
	case domain.ChannelWebhook:
		url, _ := incoming["url"].(string)
		if err := notify.ValidateWebhookURL(url); err != nil {
			return c, err
		}
		incoming["url"] = strings.TrimSpace(url)
	case domain.ChannelTelegram:
		token, _ := incoming["botToken"].(string)
		chat, _ := incoming["chatId"].(string)
		if strings.TrimSpace(token) == "" {
			return c, errors.New("the bot token is required")
		}
		if strings.TrimSpace(chat) == "" {
			return c, errors.New("the chat id is required")
		}
		incoming["botToken"] = strings.TrimSpace(token)
		incoming["chatId"] = strings.TrimSpace(chat)
	}

	encoded, err := json.Marshal(incoming)
	if err != nil {
		return c, err
	}
	c.Config = encoded
	return c, nil
}

func (a *API) createChannel(w http.ResponseWriter, r *http.Request) {
	var req channelRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	c, err := normaliseChannel(req, nil)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := a.store.CreateChannel(r.Context(), c)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, viewChannel(*created))
}

func (a *API) updateChannel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := a.store.Channel(r.Context(), id)
	if err != nil {
		a.storeErr(w, err)
		return
	}

	var req channelRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	c, err := normaliseChannel(req, existing)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := a.store.UpdateChannel(r.Context(), id, c)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, viewChannel(*updated))
}

func (a *API) deleteChannel(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteChannel(r.Context(), chi.URLParam(r, "id")); err != nil {
		a.storeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// testChannel sends one synthetic event. Configuring a webhook is otherwise a
// guess that is only disproved the next time something real happens, which may
// be days later and is exactly when you want it to work.
func (a *API) testChannel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, err := a.store.Channel(r.Context(), id)
	if err != nil {
		a.storeErr(w, err)
		return
	}

	// Deliberately synchronous and bypassing the subscription filter: the
	// operator asked for this one, and they are waiting for the answer.
	res := a.notifier.Test(r.Context(), *c, domain.Event{
		Kind:      domain.EventSettingsUpdated,
		Actor:     claimsOf(r).Username,
		Subject:   c.Name,
		Message:   "Test notification from AmneziaX",
		CreatedAt: time.Now().UTC(),
	})
	if res != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "detail": res.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "detail": "delivered"})
}

func (a *API) channelDeliveries(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.ChannelDeliveries(r.Context(), chi.URLParam(r, "id"), queryInt(r, "limit", 50))
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// eventKinds powers the subscription picker so the UI never carries its own
// copy of the list.
func (a *API) eventKinds(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, domain.AllEventKinds)
}

// ---------------------------------------------------------------- announcements

type announcementRequest struct {
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Level     string     `json:"level"`
	IsEnabled *bool      `json:"isEnabled"`
	StartsAt  *time.Time `json:"startsAt"`
	EndsAt    *time.Time `json:"endsAt"`
}

func normaliseAnnouncement(req announcementRequest) (domain.Announcement, error) {
	var a domain.Announcement
	a.Title = strings.TrimSpace(req.Title)
	a.Body = strings.TrimSpace(req.Body)
	if a.Body == "" {
		return a, errors.New("the announcement needs a body")
	}
	if len(a.Body) > 2000 {
		return a, errors.New("the announcement is too long")
	}
	if len(a.Title) > 120 {
		return a, errors.New("the title is too long")
	}

	a.Level = domain.AnnouncementLevel(strings.ToUpper(strings.TrimSpace(req.Level)))
	if a.Level == "" {
		a.Level = domain.AnnouncementInfo
	}
	if !a.Level.Valid() {
		return a, errors.New("unknown announcement level")
	}

	a.IsEnabled = true
	if req.IsEnabled != nil {
		a.IsEnabled = *req.IsEnabled
	}
	a.StartsAt, a.EndsAt = req.StartsAt, req.EndsAt
	if a.StartsAt != nil && a.EndsAt != nil && a.EndsAt.Before(*a.StartsAt) {
		return a, errors.New("the end of the window is before its start")
	}
	return a, nil
}

func (a *API) listAnnouncements(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.ListAnnouncements(r.Context())
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *API) createAnnouncement(w http.ResponseWriter, r *http.Request) {
	var req announcementRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	rec, err := normaliseAnnouncement(req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := a.store.CreateAnnouncement(r.Context(), rec)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (a *API) updateAnnouncement(w http.ResponseWriter, r *http.Request) {
	var req announcementRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	rec, err := normaliseAnnouncement(req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := a.store.UpdateAnnouncement(r.Context(), chi.URLParam(r, "id"), rec)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (a *API) deleteAnnouncement(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteAnnouncement(r.Context(), chi.URLParam(r, "id")); err != nil {
		a.storeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- node metrics

func (a *API) nodeMetrics(w http.ResponseWriter, r *http.Request) {
	hours := queryInt(r, "hours", 24)
	if hours <= 0 || hours > 24*30 {
		hours = 24
	}
	items, err := a.store.NodeMetrics(r.Context(), chi.URLParam(r, "id"),
		time.Duration(hours)*time.Hour)
	if err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "node not found")
			return
		}
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}
