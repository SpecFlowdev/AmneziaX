package httpapi

import (
	"net/http"
	"strings"

	"github.com/SpecFlowdev/AmneziaX/internal/storage/postgres"
	"github.com/go-chi/chi/v5"
)

type hostRequest struct {
	InboundID     string   `json:"inboundUuid"`
	Remark        string   `json:"remark"`
	Address       string   `json:"address"`
	Port          int      `json:"port"`
	Path          string   `json:"path"`
	SNI           string   `json:"sni"`
	HostHeader    string   `json:"hostHeader"`
	ALPN          string   `json:"alpn"`
	Fingerprint   string   `json:"fingerprint"`
	PublicKey     string   `json:"publicKey"`
	ShortID       string   `json:"shortId"`
	SpiderX       string   `json:"spiderX"`
	Flow          string   `json:"flow"`
	Security      string   `json:"security"`
	AllowInsecure bool     `json:"allowInsecure"`
	Tags          []string `json:"tags"`
	IsDisabled    bool     `json:"isDisabled"`
	ViewPosition  int      `json:"viewPosition"`
}

func (r hostRequest) toInput() (postgres.HostInput, error) {
	if strings.TrimSpace(r.InboundID) == "" {
		return postgres.HostInput{}, errBadRequest("select the inbound this host publishes")
	}
	if strings.TrimSpace(r.Address) == "" {
		return postgres.HostInput{}, errBadRequest("an address is required")
	}
	if r.Port <= 0 || r.Port > 65535 {
		return postgres.HostInput{}, errBadRequest("the port must be between 1 and 65535")
	}
	if r.Tags == nil {
		r.Tags = []string{}
	}
	return postgres.HostInput{
		InboundID:     r.InboundID,
		Remark:        r.Remark,
		Address:       strings.TrimSpace(r.Address),
		Port:          r.Port,
		Path:          r.Path,
		SNI:           r.SNI,
		HostHeader:    r.HostHeader,
		ALPN:          r.ALPN,
		Fingerprint:   r.Fingerprint,
		PublicKey:     r.PublicKey,
		ShortID:       r.ShortID,
		SpiderX:       r.SpiderX,
		Flow:          r.Flow,
		Security:      r.Security,
		AllowInsecure: r.AllowInsecure,
		Tags:          r.Tags,
		IsDisabled:    r.IsDisabled,
		ViewPosition:  r.ViewPosition,
	}, nil
}

type badRequestError string

func (e badRequestError) Error() string { return string(e) }

func errBadRequest(msg string) error { return badRequestError(msg) }

func (a *API) listHosts(w http.ResponseWriter, r *http.Request) {
	hosts, err := a.store.ListHosts(r.Context())
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, hosts)
}

func (a *API) getHost(w http.ResponseWriter, r *http.Request) {
	host, err := a.store.Host(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, host)
}

func (a *API) createHost(w http.ResponseWriter, r *http.Request) {
	var req hostRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	input, err := req.toInput()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	host, err := a.store.CreateHost(r.Context(), input)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, host)
}

func (a *API) updateHost(w http.ResponseWriter, r *http.Request) {
	var req hostRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	input, err := req.toInput()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	host, err := a.store.UpdateHost(r.Context(), chi.URLParam(r, "id"), input)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, host)
}

func (a *API) deleteHost(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteHost(r.Context(), chi.URLParam(r, "id")); err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) reorderHosts(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Order []string `json:"order"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.store.ReorderHosts(r.Context(), req.Order); err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
