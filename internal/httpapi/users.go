package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/SpecFlowdev/AmneziaX/internal/auth"
	"github.com/SpecFlowdev/AmneziaX/internal/domain"
	"github.com/SpecFlowdev/AmneziaX/internal/storage/postgres"
	"github.com/SpecFlowdev/AmneziaX/internal/subscription"
	"github.com/SpecFlowdev/AmneziaX/internal/xray"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type userRequest struct {
	Username          string                      `json:"username"`
	Status            domain.UserStatus           `json:"status"`
	TrafficLimitBytes int64                       `json:"trafficLimitBytes"`
	TrafficReset      domain.TrafficResetStrategy `json:"trafficLimitStrategy"`
	ExpireAt          *time.Time                  `json:"expireAt"`
	Description       string                      `json:"description"`
	Tag               string                      `json:"tag"`
	Email             string                      `json:"email"`
	TelegramID        *int64                      `json:"telegramId"`
	HWIDDeviceLimit   int                         `json:"hwidDeviceLimit"`
	SquadIDs          []string                    `json:"squadUuids"`
}

func (r userRequest) toInput() (postgres.UserInput, error) {
	username := strings.TrimSpace(r.Username)
	if username == "" {
		return postgres.UserInput{}, errBadRequest("a username is required")
	}
	if len(username) > 64 {
		return postgres.UserInput{}, errBadRequest("the username is too long")
	}
	if !r.Status.Valid() {
		r.Status = domain.UserActive
	}
	if !r.TrafficReset.Valid() {
		r.TrafficReset = domain.ResetNever
	}
	if r.SquadIDs == nil {
		r.SquadIDs = []string{}
	}
	return postgres.UserInput{
		Username:          username,
		Status:            r.Status,
		TrafficLimitBytes: r.TrafficLimitBytes,
		TrafficReset:      r.TrafficReset,
		ExpireAt:          r.ExpireAt,
		Description:       r.Description,
		Tag:               strings.TrimSpace(r.Tag),
		Email:             strings.TrimSpace(r.Email),
		TelegramID:        r.TelegramID,
		HWIDDeviceLimit:   r.HWIDDeviceLimit,
		SquadIDs:          r.SquadIDs,
	}, nil
}

// userView carries the subscription URL alongside the stored record so the UI
// never has to reassemble it.
type userView struct {
	*domain.User
	SubscriptionURL string `json:"subscriptionUrl"`
}

func (a *API) view(u *domain.User) userView {
	return userView{User: u, SubscriptionURL: a.subURL(u)}
}

// subURL is the single link an operator hands to a subscriber. /s/ answers both
// audiences — a browser gets the subscription page, an app gets the
// configuration — so there is only ever one address to copy, paste or scan.
// The older /sub/ form still works for links already sitting in someone's app.
func (a *API) subURL(u *domain.User) string {
	return a.cfg.SubscriptionPublicURL + "/s/" + u.SubscriptionUUID
}

func newUserSecrets() postgres.UserSecrets {
	return postgres.UserSecrets{
		ShortUUID:        auth.RandomSecret(12),
		SubscriptionUUID: uuid.NewString(),
		VlessUUID:        uuid.NewString(),
		TrojanPassword:   xray.GeneratePassword(16),
		SSPassword:       xray.GeneratePassword(32),
	}
}

func (a *API) listUsers(w http.ResponseWriter, r *http.Request) {
	f := postgres.UserFilter{
		Search:  r.URL.Query().Get("search"),
		Status:  r.URL.Query().Get("status"),
		SquadID: r.URL.Query().Get("squadUuid"),
		Tag:     r.URL.Query().Get("tag"),
		Limit:   queryInt(r, "limit", 50),
		Offset:  queryInt(r, "offset", 0),
		SortBy:  r.URL.Query().Get("sortBy"),
		Desc:    queryBool(r, "desc"),
	}
	users, total, err := a.store.ListUsers(r.Context(), f)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	out := make([]userView, 0, len(users))
	for i := range users {
		out = append(out, a.view(&users[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "total": total, "limit": f.Limit, "offset": f.Offset})
}

func (a *API) listUserTags(w http.ResponseWriter, r *http.Request) {
	tags, err := a.store.UserTags(r.Context())
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

func (a *API) getUser(w http.ResponseWriter, r *http.Request) {
	user, err := a.store.User(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.view(user))
}

func (a *API) createUser(w http.ResponseWriter, r *http.Request) {
	var req userRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	input, err := req.toInput()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	user, err := a.store.CreateUser(r.Context(), input, newUserSecrets())
	if err != nil {
		a.storeErr(w, err)
		return
	}
	a.store.LogEvent(r.Context(), domain.EventUserCreated, claimsOf(r).Username, user.Username, "user created", nil)
	a.hub.RequestSyncForUser(r.Context(), user.UUID)
	writeJSON(w, http.StatusCreated, a.view(user))
}

func (a *API) updateUser(w http.ResponseWriter, r *http.Request) {
	var req userRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	input, err := req.toInput()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	id := chi.URLParam(r, "id")
	// The user's previous squads matter too: dropping a squad must reach the
	// nodes that used to serve them.
	a.hub.RequestSyncForUser(r.Context(), id)

	user, err := a.store.UpdateUser(r.Context(), id, input)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	a.store.LogEvent(r.Context(), domain.EventUserUpdated, claimsOf(r).Username, user.Username, "user updated", nil)
	a.hub.RequestSyncForUser(r.Context(), id)
	writeJSON(w, http.StatusOK, a.view(user))
}

func (a *API) deleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	user, err := a.store.User(r.Context(), id)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	if err := a.store.DeleteUser(r.Context(), id); err != nil {
		a.storeErr(w, err)
		return
	}
	a.store.LogEvent(r.Context(), domain.EventUserDeleted, claimsOf(r).Username, user.Username, "user deleted", nil)
	a.hub.RequestSyncAll(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) enableUser(w http.ResponseWriter, r *http.Request) {
	a.setUserStatus(w, r, domain.UserActive)
}

func (a *API) disableUser(w http.ResponseWriter, r *http.Request) {
	a.setUserStatus(w, r, domain.UserDisabled)
}

func (a *API) setUserStatus(w http.ResponseWriter, r *http.Request, status domain.UserStatus) {
	id := chi.URLParam(r, "id")
	if err := a.store.SetUserStatus(r.Context(), id, status); err != nil {
		a.storeErr(w, err)
		return
	}
	a.hub.RequestSyncForUser(r.Context(), id)
	user, err := a.store.User(r.Context(), id)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.view(user))
}

func (a *API) resetUserTraffic(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := a.store.ResetUserTraffic(r.Context(), id); err != nil {
		a.storeErr(w, err)
		return
	}
	a.hub.RequestSyncForUser(r.Context(), id)
	user, err := a.store.User(r.Context(), id)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.view(user))
}

// revokeUser rolls every credential the user holds, which invalidates both the
// old subscription link and any config already imported into a client.
func (a *API) revokeUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sec := newUserSecrets()
	user, err := a.store.RevokeUserSubscription(r.Context(), id, sec.SubscriptionUUID, sec.ShortUUID,
		sec.VlessUUID, sec.TrojanPassword, sec.SSPassword)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	a.hub.RequestSyncForUser(r.Context(), id)
	writeJSON(w, http.StatusOK, a.view(user))
}

func (a *API) userUsage(w http.ResponseWriter, r *http.Request) {
	days := queryInt(r, "days", 30)
	if days <= 0 || days > 365 {
		days = 30
	}
	points, err := a.store.UserTrafficSeries(r.Context(), chi.URLParam(r, "id"),
		time.Now().Add(-time.Duration(days)*24*time.Hour))
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, points)
}

func (a *API) userLinks(w http.ResponseWriter, r *http.Request) {
	bundle, err := a.bundleFor(r, chi.URLParam(r, "id"))
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"links":           subscription.Links(bundle),
		"subscriptionUrl": a.subURL(bundle.User),
	})
}

// bulkUsers applies one action to many users, which is how an operator handles
// a whole tag or squad at once.
func (a *API) bulkUsers(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UUIDs  []string `json:"uuids"`
		Action string   `json:"action"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.UUIDs) == 0 {
		writeErr(w, http.StatusBadRequest, "select at least one user")
		return
	}

	var failed int
	for _, id := range req.UUIDs {
		var err error
		switch req.Action {
		case "enable":
			err = a.store.SetUserStatus(r.Context(), id, domain.UserActive)
		case "disable":
			err = a.store.SetUserStatus(r.Context(), id, domain.UserDisabled)
		case "reset-traffic":
			err = a.store.ResetUserTraffic(r.Context(), id)
		case "delete":
			err = a.store.DeleteUser(r.Context(), id)
		default:
			writeErr(w, http.StatusBadRequest, "unknown bulk action")
			return
		}
		if err != nil {
			failed++
			a.log.Warn("bulk action failed", "action", req.Action, "user", id, "error", err)
		}
	}
	a.hub.RequestSyncAll(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"requested": len(req.UUIDs),
		"failed":    failed,
	})
}

// bundleFor assembles everything needed to render a user's subscription.
func (a *API) bundleFor(r *http.Request, userID string) (subscription.Bundle, error) {
	user, err := a.store.User(r.Context(), userID)
	if err != nil {
		return subscription.Bundle{}, err
	}
	return a.bundleForUser(r, user)
}

func (a *API) bundleForUser(r *http.Request, user *domain.User) (subscription.Bundle, error) {
	inboundIDs, err := a.store.UserInboundIDs(r.Context(), user.UUID)
	if err != nil {
		return subscription.Bundle{}, err
	}
	hosts, err := a.store.HostsForInbounds(r.Context(), inboundIDs)
	if err != nil {
		return subscription.Bundle{}, err
	}
	// Branding is configurable at runtime, so the title clients display follows
	// the panel settings rather than the environment it booted with.
	settings, err := a.settings(r)
	if err != nil {
		return subscription.Bundle{}, err
	}
	return subscription.Bundle{
		User:       user,
		Hosts:      hosts,
		Title:      a.subscriptionTitle(settings),
		SupportURL: a.supportURL(settings),
	}, nil
}
