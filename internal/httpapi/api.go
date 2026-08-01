// Package httpapi exposes the panel's REST API and serves the web UI.
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/SpecFlowdev/AmneziaX/internal/auth"
	"github.com/SpecFlowdev/AmneziaX/internal/config"
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
}

func New(store *postgres.Store, h *hub.Hub, issuer *auth.Issuer, cfg *config.Panel, log *slog.Logger) *API {
	return &API{store: store, hub: h, issuer: issuer, cfg: cfg, log: log}
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

		r.Group(func(r chi.Router) {
			r.Use(a.authenticated)

			r.Get("/auth/me", a.me)
			r.Post("/auth/password", a.changePassword)

			r.Get("/system/overview", a.overview)
			r.Get("/system/stats/traffic", a.trafficStats)
			r.Get("/system/stats/top-users", a.topUsers)
			r.Get("/system/events", a.events)

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
		r.Method(method, "/sub/{token}", http.HandlerFunc(a.subscription))
		r.Method(method, "/sub/{token}/info", http.HandlerFunc(a.subscriptionInfo))
		r.Method(method, "/sub/{token}/links", http.HandlerFunc(a.subscriptionLinks))
		r.Method(method, "/sub/{token}/json", http.HandlerFunc(a.subscriptionJSON))
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
