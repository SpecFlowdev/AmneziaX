// Package httpapi exposes the panel's REST API and serves the web UI.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SpecFlowdev/AmneziaX/internal/auth"
	"github.com/SpecFlowdev/AmneziaX/internal/config"
	"github.com/SpecFlowdev/AmneziaX/internal/domain"
	"github.com/SpecFlowdev/AmneziaX/internal/hub"
	"github.com/SpecFlowdev/AmneziaX/internal/storage/postgres"
	"github.com/SpecFlowdev/AmneziaX/scripts"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type API struct {
	store  *postgres.Store
	hub    *hub.Hub
	issuer *auth.Issuer
	cfg    *config.Panel
	log    *slog.Logger

	// notifier delivers events outward. It is an interface so the API can be
	// exercised without a live dispatcher.
	notifier Notifier

	// Settings are read on nearly every request, including unauthenticated
	// subscription fetches, so they are cached and refreshed on write.
	settingsMu    sync.RWMutex
	settingsCache *domain.Settings
	settingsAt    time.Time
}

// Notifier is the slice of the dispatcher the HTTP layer uses: everything else
// reaches it through the event log.
type Notifier interface {
	Test(ctx context.Context, c domain.NotificationChannel, e domain.Event) error
}

func New(store *postgres.Store, h *hub.Hub, issuer *auth.Issuer, cfg *config.Panel, log *slog.Logger, notifier Notifier) *API {
	return &API{store: store, hub: h, issuer: issuer, cfg: cfg, log: log, notifier: notifier}
}

const settingsTTL = 30 * time.Second

// settings returns the panel settings, refreshing the cache when it is stale.
func (a *API) settings(r *http.Request) (*domain.Settings, error) {
	a.settingsMu.RLock()
	cached, at := a.settingsCache, a.settingsAt
	a.settingsMu.RUnlock()
	if cached != nil && time.Since(at) < settingsTTL {
		return cached, nil
	}

	fresh, err := a.store.Settings(r.Context())
	if err != nil {
		// Serving slightly stale branding beats failing the request.
		if cached != nil {
			return cached, nil
		}
		return nil, err
	}
	a.cacheSettings(fresh)
	return fresh, nil
}

// invalidateSettings drops the cache so the next read goes to the database.
// Used after a restore, which replaces the settings row underneath the cache.
func (a *API) invalidateSettings() {
	a.settingsMu.Lock()
	a.settingsCache, a.settingsAt = nil, time.Time{}
	a.settingsMu.Unlock()
}

func (a *API) cacheSettings(s *domain.Settings) {
	a.settingsMu.Lock()
	a.settingsCache, a.settingsAt = s, time.Now()
	a.settingsMu.Unlock()
}

// subscriptionTitle prefers the configured subscription title and falls back to
// the brand name, so a fresh install shows something sensible in client apps.
func (a *API) subscriptionTitle(s *domain.Settings) string {
	if s != nil {
		if s.SubscriptionTitle != "" {
			return s.SubscriptionTitle
		}
		if s.BrandName != "" {
			return s.BrandName
		}
	}
	return a.cfg.SubscriptionProfileTitle
}

func (a *API) supportURL(s *domain.Settings) string {
	if s != nil && s.SupportURL != "" {
		return s.SupportURL
	}
	return a.cfg.SubscriptionSupportURL
}

func (a *API) Router(ui http.Handler) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(a.recoverer)
	r.Use(a.cors)

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", a.health)
		r.Post("/auth/login", a.login)
		r.Get("/auth/bootstrap-status", a.bootstrapStatus)
		// Branding is public so the sign-in screen and the subscription page
		// render correctly before anyone has authenticated.
		r.Get("/branding", a.branding)

		r.Group(func(r chi.Router) {
			r.Use(a.authenticated)

			r.Get("/auth/me", a.me)
			r.Post("/auth/password", a.changePassword)

			r.Get("/system/overview", a.overview)
			r.Get("/system/stats/traffic", a.trafficStats)
			r.Get("/system/stats/top-users", a.topUsers)
			r.Get("/system/events", a.events)
			r.Get("/system/spend", a.spend)

			r.Get("/settings", a.getSettings)
			r.Put("/settings", a.writable(a.updateSettings))

			r.Route("/tokens", func(r chi.Router) {
				r.Get("/", a.ownerOnly(a.listTokens))
				r.Post("/", a.ownerOnly(a.createToken))
				r.Delete("/{id}", a.ownerOnly(a.deleteToken))
			})

			r.Route("/profiles", func(r chi.Router) {
				r.Get("/", a.listProfiles)
				r.Get("/inbounds", a.listInbounds)
				r.Post("/", a.writable(a.createProfile))
				r.Post("/tools/reality-keys", a.writable(a.realityKeys))
				r.Get("/{id}", a.getProfile)
				r.Put("/{id}", a.writable(a.updateProfile))
				r.Delete("/{id}", a.writable(a.deleteProfile))
			})

			r.Route("/nodes", func(r chi.Router) {
				r.Get("/", a.listNodes)
				r.Post("/", a.writable(a.createNode))
				r.Get("/{id}", a.getNode)
				r.Put("/{id}", a.writable(a.updateNode))
				r.Delete("/{id}", a.writable(a.deleteNode))
				r.Post("/{id}/enable", a.writable(a.enableNode))
				r.Post("/{id}/disable", a.writable(a.disableNode))
				r.Post("/{id}/restart", a.writable(a.restartNode))
				r.Post("/{id}/sync", a.writable(a.syncNode))
				r.Post("/{id}/rotate-token", a.writable(a.rotateNodeToken))
				r.Post("/{id}/reset-traffic", a.writable(a.resetNodeTraffic))
				r.Get("/{id}/config", a.previewNodeConfig)
				r.Get("/{id}/logs", a.nodeLogs)
				r.Get("/{id}/metrics", a.nodeMetrics)
			})

			r.Route("/hosts", func(r chi.Router) {
				r.Get("/", a.listHosts)
				r.Post("/", a.writable(a.createHost))
				r.Post("/reorder", a.writable(a.reorderHosts))
				r.Get("/{id}", a.getHost)
				r.Put("/{id}", a.writable(a.updateHost))
				r.Delete("/{id}", a.writable(a.deleteHost))
			})

			r.Route("/squads", func(r chi.Router) {
				r.Get("/", a.listSquads)
				r.Post("/", a.writable(a.createSquad))
				r.Get("/{id}", a.getSquad)
				r.Put("/{id}", a.writable(a.updateSquad))
				r.Delete("/{id}", a.writable(a.deleteSquad))
				r.Post("/{id}/add-all-users", a.writable(a.squadAddAll))
				r.Post("/{id}/remove-all-users", a.writable(a.squadRemoveAll))
			})

			r.Route("/users", func(r chi.Router) {
				r.Get("/", a.listUsers)
				r.Get("/tags", a.listUserTags)
				r.Post("/", a.writable(a.createUser))
				r.Post("/bulk", a.writable(a.bulkUsers))
				r.Get("/{id}", a.getUser)
				r.Put("/{id}", a.writable(a.updateUser))
				r.Delete("/{id}", a.writable(a.deleteUser))
				r.Post("/{id}/enable", a.writable(a.enableUser))
				r.Post("/{id}/disable", a.writable(a.disableUser))
				r.Post("/{id}/reset-traffic", a.writable(a.resetUserTraffic))
				r.Post("/{id}/revoke", a.writable(a.revokeUser))
				r.Get("/{id}/usage", a.userUsage)
				r.Get("/{id}/links", a.userLinks)
				r.Get("/{id}/devices", a.userDevices)
				r.Delete("/{id}/devices", a.writable(a.resetUserDevices))
				r.Delete("/{id}/devices/{hwid}", a.writable(a.deleteUserDevice))
			})

			// A snapshot carries every credential in the deployment, so both
			// halves are owner-only.
			r.Route("/rules", func(r chi.Router) {
				r.Get("/", a.listRules)
				r.Get("/test", a.testRule)
				r.Post("/", a.writable(a.createRule))
				r.Put("/{id}", a.writable(a.updateRule))
				r.Delete("/{id}", a.writable(a.deleteRule))
			})

			r.Route("/inspect", func(r chi.Router) {
				r.Get("/devices", a.inspectDevices)
				r.Get("/subscriptions", a.inspectSubscriptionRequests)
			})

			r.Route("/backup", func(r chi.Router) {
				r.Get("/", a.ownerOnly(a.backupSummary))
				r.Get("/export", a.ownerOnly(a.exportBackup))
				r.Post("/import", a.ownerOnly(a.importBackup))
			})

			r.Route("/notifications", func(r chi.Router) {
				r.Get("/events", a.eventKinds)
				r.Get("/channels", a.listChannels)
				r.Post("/channels", a.writable(a.createChannel))
				r.Put("/channels/{id}", a.writable(a.updateChannel))
				r.Delete("/channels/{id}", a.writable(a.deleteChannel))
				r.Post("/channels/{id}/test", a.writable(a.testChannel))
				r.Get("/channels/{id}/deliveries", a.channelDeliveries)
			})

			r.Route("/announcements", func(r chi.Router) {
				r.Get("/", a.listAnnouncements)
				r.Post("/", a.writable(a.createAnnouncement))
				r.Put("/{id}", a.writable(a.updateAnnouncement))
				r.Delete("/{id}", a.writable(a.deleteAnnouncement))
			})

			r.Route("/admins", func(r chi.Router) {
				r.Get("/", a.listAdmins)
				r.Post("/", a.ownerOnly(a.createAdmin))
				r.Put("/{id}", a.ownerOnly(a.updateAdmin))
				r.Delete("/{id}", a.ownerOnly(a.deleteAdmin))
			})
		})
	})

	// The node installer is fetched by a shell one-liner on a brand new server,
	// which has no credentials yet. The script itself carries no secrets — the
	// enrolment token is passed on the command line by the operator.
	r.Get("/install-node.sh", serveScript(scripts.InstallNode))
	r.Get("/install-panel.sh", serveScript(scripts.InstallPanel))

	// Prebuilt agent binaries, when the deployment ships them. This is what
	// lets install-node.sh run on a server with nothing but curl.
	if a.cfg.AgentDistDir != "" {
		if info, err := os.Stat(a.cfg.AgentDistDir); err == nil && info.IsDir() {
			r.Handle("/dist/*", http.StripPrefix("/dist/",
				http.FileServer(http.Dir(a.cfg.AgentDistDir))))
		} else {
			a.log.Info("no agent binaries bundled; nodes will build from source",
				"dir", a.cfg.AgentDistDir)
		}
	}

	// Client-facing subscription endpoints are unauthenticated by design; the
	// subscription uuid is the credential. HEAD is registered alongside GET
	// because several clients probe a subscription that way before fetching it.
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		// The one link an operator hands out. It serves the subscriber page to a
		// browser and the configuration to an app, so a subscriber can open it
		// and a client can import it without anyone choosing between two URLs.
		r.Method(method, "/s/{token}", a.subscriptionEntry(ui))

		r.Method(method, "/sub/{token}", http.HandlerFunc(a.subscription))
		r.Method(method, "/sub/{token}/info", http.HandlerFunc(a.subscriptionInfo))
		r.Method(method, "/sub/{token}/links", http.HandlerFunc(a.subscriptionLinks))
		r.Method(method, "/sub/{token}/json", http.HandlerFunc(a.subscriptionJSON))
		r.Method(method, "/sub/{token}/clash", http.HandlerFunc(a.subscriptionClash))
		r.Method(method, "/sub/{token}/singbox", http.HandlerFunc(a.subscriptionSingBox))
	}

	if ui != nil {
		r.NotFound(ui.ServeHTTP)
	}
	return r
}

// serveScript hands out an embedded installer as a plain shell script.
func serveScript(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte(body))
	}
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// ---------------------------------------------------------------- helpers

type errorBody struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg})
}

// storeErr maps persistence errors onto HTTP statuses so handlers stay short.
func (a *API) storeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, postgres.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	case errors.Is(err, postgres.ErrConflict):
		writeErr(w, http.StatusConflict, "a record with this name already exists")
	default:
		a.log.Error("request failed", "error", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
	}
}

func decode(r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errors.New("invalid request body: " + err.Error())
	}
	return nil
}

func queryInt(r *http.Request, key string, def int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get(key)); err == nil {
		return v
	}
	return def
}

func queryBool(r *http.Request, key string) bool {
	v, _ := strconv.ParseBool(r.URL.Query().Get(key))
	return v
}

func (a *API) cors(next http.Handler) http.Handler {
	allowed := map[string]bool{}
	wildcard := false
	for _, o := range a.cfg.CORSOrigins {
		if o == "*" {
			wildcard = true
		}
		allowed[o] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (wildcard || allowed[origin]) {
			if wildcard {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				a.log.Error("panic in handler", "path", r.URL.Path, "panic", rec)
				writeErr(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}
