package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
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
	// Either a six digit code from the app, or one recovery code. Sent on the
	// second call, after the first answered totpRequired.
	Code string `json:"code"`
}

type loginResponse struct {
	Token     string        `json:"token"`
	ExpiresAt time.Time     `json:"expiresAt"`
	Admin     *domain.Admin `json:"admin"`

	// TOTPRequired asks the sign-in form for a code and nothing else — the
	// password was right, but it is not a session yet.
	TOTPRequired bool `json:"totpRequired,omitempty"`
	// EnrolTOTP means the panel requires two-factor and this account has none,
	// so the only way forward is to set one up.
	EnrolTOTP bool `json:"enrolTotp,omitempty"`
	// RecoveryCodesLeft warns after a recovery code was spent.
	RecoveryCodesLeft *int `json:"recoveryCodesLeft,omitempty"`
}

// throttleKeys are the two counters a sign-in attempt is charged to. Prefixes
// keep a username that looks like an address from colliding with one.
func throttleKeys(r *http.Request, username string) []string {
	return []string{
		"user:" + strings.ToLower(strings.TrimSpace(username)),
		"ip:" + clientIP(r),
	}
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	keys := throttleKeys(r, req.Username)

	// Refuse before touching the database, so a locked-out attacker cannot use
	// the endpoint to measure anything at all.
	if locked, left := a.throttle.Locked(keys...); locked {
		w.Header().Set("Retry-After", strconv.Itoa(int(left.Seconds())+1))
		writeErr(w, http.StatusTooManyRequests, "too many attempts, try again in "+humaniseWait(left))
		return
	}

	admin, err := a.store.AdminByUsername(r.Context(), req.Username)
	if err != nil || !auth.CheckPassword(admin.PasswordHash, req.Password) || admin.IsDisabled {
		// A uniform delay and message keep the endpoint from confirming which
		// usernames exist.
		time.Sleep(300 * time.Millisecond)
		a.failedLogin(r, keys, req.Username, "failed sign-in attempt")
		writeErr(w, http.StatusUnauthorized, "invalid username or password")
		return
	}

	settings, err := a.store.Settings(r.Context())
	if err != nil {
		a.storeErr(w, err)
		return
	}

	// The password is right from here on. What is left is the second factor, so
	// the answers stop being "invalid username or password".
	switch {
	case admin.TOTPEnabled:
		if req.Code == "" {
			writeJSON(w, http.StatusOK, loginResponse{TOTPRequired: true})
			return
		}
		left, ok := a.verifySecondFactor(r, admin, req.Code)
		if !ok {
			a.failedLogin(r, keys, admin.Username, "failed two-factor code")
			writeErr(w, http.StatusUnauthorized, "that code is not valid")
			return
		}
		a.throttle.Succeed(keys...)
		a.issueSession(w, r, admin, left)
		return

	case settings.RequireTOTP:
		// Required panel-wide and this account has none. Enrolment needs a
		// session, so one is issued — but the UI is told to go straight to
		// setting up the second factor.
		a.throttle.Succeed(keys...)
		token, expires, err := a.issuer.Issue(admin)
		if err != nil {
			a.storeErr(w, err)
			return
		}
		a.store.TouchAdminLogin(r.Context(), admin.UUID)
		a.store.LogEvent(r.Context(), domain.EventAdminLogin, admin.Username, "",
			"signed in — two-factor enrolment required", nil)
		writeJSON(w, http.StatusOK, loginResponse{
			Token: token, ExpiresAt: expires, Admin: admin, EnrolTOTP: true,
		})
		return

	default:
		a.throttle.Succeed(keys...)
		a.issueSession(w, r, admin, nil)
	}
}

// verifySecondFactor accepts either a current TOTP code or one recovery code,
// and returns how many recovery codes are left when one was spent.
func (a *API) verifySecondFactor(r *http.Request, admin *domain.Admin, code string) (*int, bool) {
	if auth.CheckTOTP(admin.TOTPSecret, code) {
		// A valid code still has to be unused. Without this, a code read over
		// someone's shoulder works for the rest of its thirty seconds.
		fresh, err := a.store.ClaimTOTPStep(r.Context(), admin.UUID, auth.TOTPStep(time.Now()))
		if err != nil || !fresh {
			return nil, false
		}
		return nil, true
	}

	if idx := auth.MatchRecoveryCode(admin.RecoveryCodeHashes, code); idx >= 0 {
		used, err := a.store.ConsumeRecoveryCode(r.Context(), admin.UUID, admin.RecoveryCodeHashes[idx])
		if err != nil || !used {
			return nil, false
		}
		left := len(admin.RecoveryCodeHashes) - 1
		a.store.LogEvent(r.Context(), domain.EventAdminLogin, admin.Username, "",
			"signed in with a recovery code", map[string]any{"remaining": left})
		return &left, true
	}
	return nil, false
}

func (a *API) issueSession(w http.ResponseWriter, r *http.Request, admin *domain.Admin, recoveryLeft *int) {
	token, expires, err := a.issuer.Issue(admin)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	a.store.TouchAdminLogin(r.Context(), admin.UUID)
	a.store.LogEvent(r.Context(), domain.EventAdminLogin, admin.Username, "", "signed in", nil)
	writeJSON(w, http.StatusOK, loginResponse{
		Token: token, ExpiresAt: expires, Admin: admin, RecoveryCodesLeft: recoveryLeft,
	})
}

// failedLogin charges the attempt to both counters and records it, noting in the
// event log when the failure was the one that locked the account.
func (a *API) failedLogin(r *http.Request, keys []string, username, reason string) {
	locked := a.throttle.Fail(keys...)
	meta := map[string]any{"ip": clientIP(r)}
	if locked > 0 {
		meta["lockedFor"] = locked.String()
		reason += " — further attempts locked for " + humaniseWait(locked)
	}
	a.store.LogEvent(r.Context(), domain.EventAdminLoginFailed, username, "", reason, meta)

	// A lockout is the signal worth waking someone up for: repeated failures
	// against a panel that holds every credential in the deployment.
	if locked > 0 {
		a.store.LogEvent(r.Context(), domain.EventAdminLocked, username, "",
			"sign-in locked after repeated failures", meta)
	}
}

func humaniseWait(d time.Duration) string {
	if d < time.Minute {
		return strconv.Itoa(int(d.Seconds())+1) + "s"
	}
	return strconv.Itoa(int(d.Minutes())+1) + "m"
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
