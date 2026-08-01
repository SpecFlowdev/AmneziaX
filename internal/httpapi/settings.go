package httpapi

import (
	"net/http"
	"strings"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
)

// maxLogoBytes caps the inline logo. Logos are stored as data URIs so a
// deployment needs no object storage, but they must stay small enough to sit in
// a row and load on every page.
const maxLogoBytes = 256 * 1024

// brandingResponse is the unauthenticated slice of the settings: everything the
// login screen and the subscription page need to look right before anyone has
// signed in.
type brandingResponse struct {
	BrandName    string `json:"brandName"`
	BrandTagline string `json:"brandTagline"`
	BrandLogo    string `json:"brandLogo"`
	BrandAccent  string `json:"brandAccent"`
	SupportURL   string `json:"supportUrl"`
}

func (a *API) branding(w http.ResponseWriter, r *http.Request) {
	s, err := a.settings(r)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, brandingResponse{
		BrandName:    s.BrandName,
		BrandTagline: s.BrandTagline,
		BrandLogo:    s.BrandLogo,
		BrandAccent:  s.BrandAccent,
		SupportURL:   s.SupportURL,
	})
}

func (a *API) getSettings(w http.ResponseWriter, r *http.Request) {
	s, err := a.settings(r)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (a *API) updateSettings(w http.ResponseWriter, r *http.Request) {
	var req domain.Settings
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	req.BrandName = strings.TrimSpace(req.BrandName)
	if req.BrandName == "" {
		req.BrandName = "AmneziaX"
	}
	if len(req.BrandName) > 64 {
		writeErr(w, http.StatusBadRequest, "the panel name is too long")
		return
	}
	req.BrandLogo = strings.TrimSpace(req.BrandLogo)
	if len(req.BrandLogo) > maxLogoBytes {
		writeErr(w, http.StatusBadRequest, "the logo is too large — use an image under 180 KB")
		return
	}
	// Only inline images and same-origin paths, so a logo can never pull a
	// script or beacon a third party from every admin's browser.
	if req.BrandLogo != "" &&
		!strings.HasPrefix(req.BrandLogo, "data:image/") &&
		!strings.HasPrefix(req.BrandLogo, "/") {
		writeErr(w, http.StatusBadRequest, "the logo must be an uploaded image")
		return
	}
	req.BrandAccent = strings.TrimSpace(req.BrandAccent)
	if req.BrandAccent != "" && !isHexColour(req.BrandAccent) {
		writeErr(w, http.StatusBadRequest, "the accent colour must be a hex value like #e11d48")
		return
	}
	req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
	if req.Currency == "" {
		req.Currency = "USD"
	}
	if len(req.Currency) > 8 {
		writeErr(w, http.StatusBadRequest, "the currency code is too long")
		return
	}

	updated, err := a.store.UpdateSettings(r.Context(), req)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	a.cacheSettings(updated)
	a.store.LogEvent(r.Context(), domain.EventSettingsUpdated, claimsOf(r).Username, "",
		"panel settings updated", nil)
	writeJSON(w, http.StatusOK, updated)
}

func isHexColour(v string) bool {
	if len(v) != 4 && len(v) != 7 {
		return false
	}
	if v[0] != '#' {
		return false
	}
	for _, c := range v[1:] {
		isHexDigit := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
		if !isHexDigit {
			return false
		}
	}
	return true
}
