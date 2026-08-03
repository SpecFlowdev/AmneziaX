package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
	"github.com/SpecFlowdev/AmneziaX/internal/storage/postgres"
	"github.com/SpecFlowdev/AmneziaX/internal/subscription"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// resolveSubscriber accepts either the subscription uuid or the shorter link id,
// so an operator can hand out whichever form fits the channel.
func (a *API) resolveSubscriber(r *http.Request) (*domain.User, error) {
	token := strings.TrimSpace(chi.URLParam(r, "token"))
	if token == "" {
		return nil, postgres.ErrNotFound
	}
	// The subscription column is a uuid, so anything that is not one has to
	// skip that lookup — otherwise Postgres rejects the parameter outright and
	// a mistyped link surfaces as a server error instead of a 404.
	if _, err := uuid.Parse(token); err == nil {
		user, err := a.store.UserBySubscription(r.Context(), token)
		if err == nil {
			return user, nil
		}
		if !errors.Is(err, postgres.ErrNotFound) {
			return nil, err
		}
	}
	return a.store.UserByShortUUID(r.Context(), token)
}

func (a *API) subscriptionBundle(w http.ResponseWriter, r *http.Request) (subscription.Bundle, bool) {
	user, err := a.resolveSubscriber(r)
	if err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			// Recorded rather than dropped: a token that resolves to nobody is
			// the most interesting entry in the log — a revoked link still
			// being polled, or somebody guessing.
			a.logSubRequest(r, nil, http.StatusNotFound)
			writeErr(w, http.StatusNotFound, "subscription not found")
		} else {
			a.storeErr(w, err)
		}
		return subscription.Bundle{}, false
	}
	if user.Status == domain.UserDisabled {
		a.logSubRequest(r, user, http.StatusForbidden)
		writeErr(w, http.StatusForbidden, "this subscription is disabled")
		return subscription.Bundle{}, false
	}

	// Clients that identify their device let the panel enforce the device limit.
	// Everything else is recorded but never blocked, because a missing header is
	// not evidence of a new device.
	if hwid := deviceID(r); hwid != "" {
		allowed, _, err := a.store.TouchDevice(r.Context(), user.UUID, hwid,
			r.UserAgent(), r.Header.Get("X-Device-OS"), user.HWIDDeviceLimit)
		if err != nil {
			a.log.Warn("device tracking failed", "user", user.UUID, "error", err)
		} else if !allowed {
			a.store.LogEvent(r.Context(), domain.EventDeviceBlocked, "subscription", user.Username,
				"device limit reached", map[string]any{"limit": user.HWIDDeviceLimit})
			writeErr(w, http.StatusForbidden, "this account has reached its device limit")
			return subscription.Bundle{}, false
		}
	}

	a.store.TouchSubscriptionOpen(r.Context(), user.UUID, r.UserAgent())
	a.logSubRequest(r, user, http.StatusOK)

	bundle, err := a.bundleForUser(r, user)
	if err != nil {
		a.storeErr(w, err)
		return subscription.Bundle{}, false
	}
	return bundle, true
}

// deviceID reads the hardware id clients send under one of several header
// names; they have not converged on one.
func deviceID(r *http.Request) string {
	for _, h := range []string{"X-Hwid", "X-HWID", "X-Device-Id", "Hwid"} {
		if v := strings.TrimSpace(r.Header.Get(h)); v != "" {
			if len(v) > 128 {
				v = v[:128]
			}
			return v
		}
	}
	return ""
}

func applySubHeaders(w http.ResponseWriter, b subscription.Bundle) {
	for k, v := range subscription.Headers(b) {
		w.Header().Set(k, v)
	}
}

// requestedFormat decides what a subscription request is answered with, in
// descending order of how specific the instruction is:
//
//  1. ?format= on the URL — whoever wrote the link said exactly what they want.
//  2. The client's own format, when it announces itself. A Clash app is handed
//     Clash whatever the panel prefers; anything else is simply broken.
//  3. An operator's response rule matching the User-Agent. This sits below the
//     built-in detection deliberately: a rule is for clients the panel does not
//     recognise, and letting one override a known Clash client would only ever
//     break it.
//  4. The panel's configured default, for everything else. Empty leaves the
//     base64 list, which every client can read.
func (a *API) requestedFormat(r *http.Request) subscription.Format {
	return a.formatFor(r, true)
}

// formatFor is requestedFormat with the hit counter made optional. The count
// has to happen exactly once per request: the answer is needed twice — once to
// serve and once to log — and a preview must not count at all, or the number
// that tells an operator whether a rule ever fires is the one thing they
// cannot trust.
func (a *API) formatFor(r *http.Request, count bool) subscription.Format {
	if raw := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format"))); raw != "" {
		if f := subscription.Format(raw); f.Valid() {
			return f
		}
	}
	if f, ok := subscription.DetectClientFormat(r.UserAgent()); ok {
		return f
	}
	if raw, ok := a.matchRule(r, count); ok {
		if f := subscription.Format(strings.ToLower(strings.TrimSpace(raw))); f.Valid() {
			return f
		}
	}
	if s, err := a.settings(r); err == nil {
		if f := subscription.Format(strings.ToLower(strings.TrimSpace(s.SubscriptionFormat))); f.Valid() {
			return f
		}
	}
	return subscription.FormatBase64
}

// subscriptionEntry backs /s/{token}, the only link an operator needs to hand
// out. A person opening it in a browser gets the subscription page; a client app
// fetching the same URL gets the configuration.
func (a *API) subscriptionEntry(ui http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ui != nil && wantsHTML(r) {
			ui.ServeHTTP(w, r)
			return
		}
		a.subscription(w, r)
	}
}

// wantsHTML separates a browser from a client app. Browsers ask for text/html
// when they navigate; subscription clients send */* or no Accept at all. Keying
// on that rather than on a list of known User-Agents means a client nobody has
// seen yet still receives a configuration instead of a page it cannot parse.
//
// ?format= settles it either way: an explicit encoding always wins, and
// ?format=page forces the human view for anyone debugging a link.
func wantsHTML(r *http.Request) bool {
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format"))) {
	case "":
	case "page", "html":
		return true
	default:
		return false
	}
	for _, part := range strings.Split(r.Header.Get("Accept"), ",") {
		media := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if strings.EqualFold(media, "text/html") {
			return true
		}
	}
	return false
}

// subscription serves the payload in whatever encoding the client understands.
func (a *API) subscription(w http.ResponseWriter, r *http.Request) {
	bundle, ok := a.subscriptionBundle(w, r)
	if !ok {
		return
	}
	format := a.requestedFormat(r)
	applySubHeaders(w, bundle)
	w.Header().Set("Content-Type", format.ContentType())
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(subscription.RenderWith(bundle, format, a.templates(r))))
}

// subscriptionLinks returns the plain list, which is easier to paste manually
// and to debug.
func (a *API) subscriptionLinks(w http.ResponseWriter, r *http.Request) {
	bundle, ok := a.subscriptionBundle(w, r)
	if !ok {
		return
	}
	applySubHeaders(w, bundle)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(strings.Join(subscription.Links(bundle), "\n")))
}

// subscriptionJSON serves the Xray "JSON subscription": an array of complete
// client configurations. It used to return the same account summary as /info,
// which made it a duplicate rather than something a client could import.
func (a *API) subscriptionJSON(w http.ResponseWriter, r *http.Request) {
	a.serveFormat(w, r, subscription.FormatJSON)
}

// subscriptionInfo powers the human-facing subscription page.
func (a *API) subscriptionInfo(w http.ResponseWriter, r *http.Request) {
	bundle, ok := a.subscriptionBundle(w, r)
	if !ok {
		return
	}
	info := subscription.BuildInfo(bundle, a.subURL(bundle.User))

	// Defaults stay on if the settings cannot be read: a page missing its
	// links is a worse failure than one showing them.
	info.ShowLinks, info.ShowFormats = true, true
	if s, err := a.settings(r); err == nil && s != nil {
		info.ShowLinks, info.ShowFormats = s.SubPageShowLinks, s.SubPageShowFormats
		if !info.ShowLinks {
			info.Links = nil
		}
	}

	// Notices are best-effort: a subscriber who cannot see this month's
	// maintenance note is a smaller problem than one who cannot fetch their
	// configuration at all, so a failure here never fails the request.
	if notices, err := a.store.LiveAnnouncements(r.Context()); err != nil {
		a.log.Warn("cannot load announcements", "error", err)
	} else {
		info.Announcements = notices
	}

	writeJSON(w, http.StatusOK, info)
}

// subscriptionClash and subscriptionSingBox let an operator hand out a link
// that pins the format, for clients that do not identify themselves.
func (a *API) subscriptionClash(w http.ResponseWriter, r *http.Request) {
	a.serveFormat(w, r, subscription.FormatClash)
}

func (a *API) subscriptionSingBox(w http.ResponseWriter, r *http.Request) {
	a.serveFormat(w, r, subscription.FormatSingBox)
}

func (a *API) serveFormat(w http.ResponseWriter, r *http.Request, f subscription.Format) {
	bundle, ok := a.subscriptionBundle(w, r)
	if !ok {
		return
	}
	applySubHeaders(w, bundle)
	w.Header().Set("Content-Type", f.ContentType())
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(subscription.RenderWith(bundle, f, a.templates(r))))
}

// logSubRequest records one fetch of a subscription link. It is best-effort by
// construction: a subscriber getting their configuration must never fail
// because the audit write did.
func (a *API) logSubRequest(r *http.Request, user *domain.User, status int) {
	rec := postgres.SubscriptionRequest{
		Token:     strings.TrimSpace(chi.URLParam(r, "token")),
		IP:        clientIP(r),
		UserAgent: r.UserAgent(),
		// Preview, not the counting path: this request has already been
		// resolved once, and asking again would count the same hit twice.
		Format: string(a.formatFor(r, false)),
		Status: status,
		HWID:   deviceID(r),
	}
	if user != nil {
		id := user.UUID
		rec.UserID = &id
	}
	a.store.RecordSubscriptionRequest(r.Context(), rec)
}

// clientIP prefers what the reverse proxy reports, since the panel only ever
// sees Caddy's own address otherwise.
func clientIP(r *http.Request) string {
	for _, h := range []string{"X-Forwarded-For", "X-Real-Ip"} {
		if v := strings.TrimSpace(r.Header.Get(h)); v != "" {
			if comma := strings.Index(v, ","); comma > 0 {
				v = strings.TrimSpace(v[:comma])
			}
			return v
		}
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return host
}

// matchRule consults the operator's rules, counting the hit only when asked.
func (a *API) matchRule(r *http.Request, count bool) (string, bool) {
	if count {
		return a.store.MatchRule(r.Context(), r.UserAgent())
	}
	return a.store.PreviewRule(r.Context(), r.UserAgent())
}

// templates loads the operator's custom documents. A settings failure falls
// back to the built-in rendering rather than to nothing.
func (a *API) templates(r *http.Request) subscription.Templates {
	s, err := a.settings(r)
	if err != nil || s == nil {
		return subscription.Templates{}
	}
	return subscription.Templates{Clash: s.ClashTemplate, SingBox: s.SingBoxTemplate}
}
