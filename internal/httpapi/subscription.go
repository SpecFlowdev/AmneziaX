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
			writeErr(w, http.StatusNotFound, "subscription not found")
		} else {
			a.storeErr(w, err)
		}
		return subscription.Bundle{}, false
	}
	if user.Status == domain.UserDisabled {
		writeErr(w, http.StatusForbidden, "this subscription is disabled")
		return subscription.Bundle{}, false
	}
	a.store.TouchSubscriptionOpen(r.Context(), user.UUID, r.UserAgent())

	bundle, err := a.bundleForUser(r, user)
	if err != nil {
		a.storeErr(w, err)
		return subscription.Bundle{}, false
	}
	return bundle, true
}

func applySubHeaders(w http.ResponseWriter, b subscription.Bundle) {
	for k, v := range subscription.Headers(b) {
		w.Header().Set(k, v)
	}
}

// subscription returns the base64 payload every mainstream client understands.
func (a *API) subscription(w http.ResponseWriter, r *http.Request) {
	bundle, ok := a.subscriptionBundle(w, r)
	if !ok {
		return
	}
	applySubHeaders(w, bundle)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(subscription.Base64(bundle)))
}

// subscriptionLinks returns the same list in plain text, which is easier to
// paste manually and to debug.
func (a *API) subscriptionLinks(w http.ResponseWriter, r *http.Request) {
	bundle, ok := a.subscriptionBundle(w, r)
	if !ok {
		return
	}
	applySubHeaders(w, bundle)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(strings.Join(subscription.Links(bundle), "\n")))
}

func (a *API) subscriptionJSON(w http.ResponseWriter, r *http.Request) {
	bundle, ok := a.subscriptionBundle(w, r)
	if !ok {
		return
	}
	applySubHeaders(w, bundle)
	writeJSON(w, http.StatusOK, subscription.BuildInfo(bundle, a.subURL(bundle.User)))
}

// subscriptionInfo powers the human-facing subscription page.
func (a *API) subscriptionInfo(w http.ResponseWriter, r *http.Request) {
	bundle, ok := a.subscriptionBundle(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, subscription.BuildInfo(bundle, a.subURL(bundle.User)))
}
