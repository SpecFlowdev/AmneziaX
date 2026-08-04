package httpapi

import (
	"net/http"
	"time"

	"github.com/SpecFlowdev/AmneziaX/internal/auth"
	"github.com/SpecFlowdev/AmneziaX/internal/domain"
	"github.com/go-chi/chi/v5"
)

// Enrolment is deliberately two calls. The first hands back a secret and its
// QR; the second only turns it on once a code proves the app actually holds it.
// Doing it in one step would let someone scan into an app that never worked and
// lock themselves out of their own panel.

type totpStatusResponse struct {
	Enabled           bool       `json:"enabled"`
	ConfirmedAt       *time.Time `json:"confirmedAt"`
	RecoveryCodesLeft int        `json:"recoveryCodesLeft"`
	// RequiredByPanel tells the UI the account cannot opt out.
	RequiredByPanel bool `json:"requiredByPanel"`
}

func (a *API) totpStatus(w http.ResponseWriter, r *http.Request) {
	admin, err := a.store.AdminByUUID(r.Context(), claimsOf(r).AdminID)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	settings, err := a.settings(r)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, totpStatusResponse{
		Enabled:           admin.TOTPEnabled,
		ConfirmedAt:       admin.TOTPConfirmedAt,
		RecoveryCodesLeft: admin.RecoveryCodesLeft,
		RequiredByPanel:   settings.RequireTOTP,
	})
}

// totpStart issues a fresh secret. Calling it again before confirming replaces
// the staged secret, which is what an operator who scanned a stale QR needs.
func (a *API) totpStart(w http.ResponseWriter, r *http.Request) {
	admin, err := a.store.AdminByUUID(r.Context(), claimsOf(r).AdminID)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	if admin.TOTPEnabled {
		writeErr(w, http.StatusConflict, "two-factor is already on for this account")
		return
	}
	secret, err := auth.NewTOTPSecret()
	if err != nil {
		a.storeErr(w, err)
		return
	}
	if err := a.store.StageTOTPSecret(r.Context(), admin.UUID, secret); err != nil {
		a.storeErr(w, err)
		return
	}

	settings, err := a.settings(r)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	issuer := settings.BrandName
	if issuer == "" {
		issuer = "AmneziaX"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"secret": secret,
		"uri":    auth.TOTPURI(issuer, admin.Username, secret),
	})
}

type totpConfirmRequest struct {
	Code string `json:"code"`
}

// totpConfirm turns the staged secret on and hands back the recovery codes.
// This is the only time they exist in readable form.
func (a *API) totpConfirm(w http.ResponseWriter, r *http.Request) {
	var req totpConfirmRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	admin, err := a.store.AdminByUUID(r.Context(), claimsOf(r).AdminID)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	if admin.TOTPEnabled {
		writeErr(w, http.StatusConflict, "two-factor is already on for this account")
		return
	}
	if admin.TOTPSecret == "" {
		writeErr(w, http.StatusBadRequest, "start the setup first")
		return
	}
	if !auth.CheckTOTP(admin.TOTPSecret, req.Code) {
		writeErr(w, http.StatusBadRequest, "that code is not valid — check the clock on your phone")
		return
	}

	plain, digests, err := auth.NewRecoveryCodes()
	if err != nil {
		a.storeErr(w, err)
		return
	}
	// The confirming code is burned with the same step counter the sign-in path
	// uses, so it cannot immediately be replayed as a first login.
	if err := a.store.ConfirmTOTP(r.Context(), admin.UUID, auth.TOTPStep(time.Now()), digests); err != nil {
		a.storeErr(w, err)
		return
	}
	a.store.LogEvent(r.Context(), domain.EventAdminSecurity, claimsOf(r).Username, admin.Username,
		"two-factor authentication enabled", nil)

	writeJSON(w, http.StatusOK, map[string]any{"recoveryCodes": plain})
}

type totpDisableRequest struct {
	Password string `json:"password"`
}

// totpDisable needs the password again. A session left open on an unlocked
// machine should not be enough to take the second factor off.
func (a *API) totpDisable(w http.ResponseWriter, r *http.Request) {
	var req totpDisableRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	admin, err := a.store.AdminByUUID(r.Context(), claimsOf(r).AdminID)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	if !auth.CheckPassword(admin.PasswordHash, req.Password) {
		time.Sleep(300 * time.Millisecond)
		writeErr(w, http.StatusUnauthorized, "wrong password")
		return
	}
	settings, err := a.settings(r)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	if settings.RequireTOTP {
		writeErr(w, http.StatusForbidden, "the panel requires two-factor for every administrator")
		return
	}
	if err := a.store.DisableTOTP(r.Context(), admin.UUID); err != nil {
		a.storeErr(w, err)
		return
	}
	a.store.LogEvent(r.Context(), domain.EventAdminSecurity, claimsOf(r).Username, admin.Username,
		"two-factor authentication disabled", nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// totpRecoveryCodes issues a fresh set, invalidating the previous one. Also
// password-gated: the codes are a way past the second factor.
func (a *API) totpRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	var req totpDisableRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	admin, err := a.store.AdminByUUID(r.Context(), claimsOf(r).AdminID)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	if !auth.CheckPassword(admin.PasswordHash, req.Password) {
		time.Sleep(300 * time.Millisecond)
		writeErr(w, http.StatusUnauthorized, "wrong password")
		return
	}
	if !admin.TOTPEnabled {
		writeErr(w, http.StatusBadRequest, "two-factor is not on for this account")
		return
	}
	plain, digests, err := auth.NewRecoveryCodes()
	if err != nil {
		a.storeErr(w, err)
		return
	}
	if err := a.store.ReplaceRecoveryCodes(r.Context(), admin.UUID, digests); err != nil {
		a.storeErr(w, err)
		return
	}
	a.store.LogEvent(r.Context(), domain.EventAdminSecurity, claimsOf(r).Username, admin.Username,
		"recovery codes regenerated", nil)
	writeJSON(w, http.StatusOK, map[string]any{"recoveryCodes": plain})
}

// totpResetForAdmin is the owner's way to unstick someone who lost both their
// phone and their recovery codes. It clears the factor rather than revealing
// anything, so the account owner must enrol again.
func (a *API) totpResetForAdmin(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	target, err := a.store.AdminByUUID(r.Context(), id)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	if err := a.store.DisableTOTP(r.Context(), target.UUID); err != nil {
		a.storeErr(w, err)
		return
	}
	a.store.LogEvent(r.Context(), domain.EventAdminSecurity, claimsOf(r).Username, target.Username,
		"two-factor reset by the owner", nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
