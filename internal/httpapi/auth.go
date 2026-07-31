package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/SpecFlowdev/AmneziaX/internal/auth"
	"github.com/SpecFlowdev/AmneziaX/internal/domain"
	"github.com/SpecFlowdev/AmneziaX/internal/storage/postgres"
	"github.com/go-chi/chi/v5"
)

type ctxKey string

const claimsKey ctxKey = "claims"

func claimsOf(r *http.Request) *auth.Claims {
	c, _ := r.Context().Value(claimsKey).(*auth.Claims)
	return c
}

func (a *API) authenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			writeErr(w, http.StatusUnauthorized, "authorization required")
			return
		}
		claims, err := a.issuer.Parse(token)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "session expired, sign in again")
			return
		}
		admin, err := a.store.AdminByUUID(r.Context(), claims.AdminID)
		if err != nil || admin.IsDisabled {
			writeErr(w, http.StatusUnauthorized, "account is no longer active")
			return
		}
		// Trust the stored role over the token so a demotion takes effect
		// immediately instead of at the next sign-in.
		claims.Role = admin.Role
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claimsKey, claims)))
	})
}

// writable rejects mutating requests from read-only accounts.
func (a *API) writable(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := claimsOf(r)
		if c == nil || !c.Role.CanWrite() {
			writeErr(w, http.StatusForbidden, "your role cannot modify this resource")
			return
		}
		h(w, r)
	}
}

func (a *API) ownerOnly(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := claimsOf(r)
		if c == nil || c.Role != domain.RoleOwner {
			writeErr(w, http.StatusForbidden, "only the owner can manage administrators")
			return
		}
		h(w, r)
	}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string        `json:"token"`
	ExpiresAt time.Time     `json:"expiresAt"`
	Admin     *domain.Admin `json:"admin"`
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	admin, err := a.store.AdminByUsername(r.Context(), req.Username)
	if err != nil || !auth.CheckPassword(admin.PasswordHash, req.Password) || admin.IsDisabled {
		// A uniform delay and message keep the endpoint from confirming which
		// usernames exist.
		time.Sleep(300 * time.Millisecond)
		a.store.LogEvent(r.Context(), domain.EventAdminLoginFailed, req.Username, "", "failed sign-in attempt", nil)
		writeErr(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	token, expires, err := a.issuer.Issue(admin)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	a.store.TouchAdminLogin(r.Context(), admin.UUID)
	a.store.LogEvent(r.Context(), domain.EventAdminLogin, admin.Username, "", "signed in", nil)
	writeJSON(w, http.StatusOK, loginResponse{Token: token, ExpiresAt: expires, Admin: admin})
}

func (a *API) bootstrapStatus(w http.ResponseWriter, r *http.Request) {
	count, err := a.store.CountAdmins(r.Context())
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"initialized": count > 0})
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	admin, err := a.store.AdminByUUID(r.Context(), claimsOf(r).AdminID)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, admin)
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func (a *API) changePassword(w http.ResponseWriter, r *http.Request) {
	var req changePasswordRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.NewPassword) < 8 {
		writeErr(w, http.StatusBadRequest, "the new password must be at least 8 characters long")
		return
	}
	admin, err := a.store.AdminByUUID(r.Context(), claimsOf(r).AdminID)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	if !auth.CheckPassword(admin.PasswordHash, req.CurrentPassword) {
		writeErr(w, http.StatusForbidden, "the current password is incorrect")
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	if err := a.store.UpdateAdminPassword(r.Context(), admin.UUID, hash); err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) listAdmins(w http.ResponseWriter, r *http.Request) {
	admins, err := a.store.ListAdmins(r.Context())
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, admins)
}

type adminRequest struct {
	Username   string           `json:"username"`
	Password   string           `json:"password"`
	Role       domain.AdminRole `json:"role"`
	IsDisabled bool             `json:"isDisabled"`
}

func (a *API) createAdmin(w http.ResponseWriter, r *http.Request) {
	var req adminRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Username == "" || len(req.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "a username and a password of at least 8 characters are required")
		return
	}
	if !req.Role.Valid() {
		req.Role = domain.RoleAdmin
	}
	if req.Role == domain.RoleOwner {
		writeErr(w, http.StatusBadRequest, "there can only be one owner account")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	admin, err := a.store.CreateAdmin(r.Context(), req.Username, hash, req.Role)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, admin)
}

func (a *API) updateAdmin(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req adminRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if !req.Role.Valid() {
		writeErr(w, http.StatusBadRequest, "unknown role")
		return
	}
	if id == claimsOf(r).AdminID && (req.IsDisabled || req.Role != domain.RoleOwner) {
		writeErr(w, http.StatusBadRequest, "you cannot lock yourself out of the panel")
		return
	}
	admin, err := a.store.UpdateAdmin(r.Context(), id, req.Role, req.IsDisabled)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	if req.Password != "" {
		if len(req.Password) < 8 {
			writeErr(w, http.StatusBadRequest, "the password must be at least 8 characters long")
			return
		}
		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			a.storeErr(w, err)
			return
		}
		if err := a.store.UpdateAdminPassword(r.Context(), id, hash); err != nil {
			a.storeErr(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, admin)
}

func (a *API) deleteAdmin(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == claimsOf(r).AdminID {
		writeErr(w, http.StatusBadRequest, "you cannot delete your own account")
		return
	}
	if err := a.store.DeleteAdmin(r.Context(), id); err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
