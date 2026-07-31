// Package webui serves the compiled single-page application from the panel
// binary, so a deployment is one file plus a database.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var assets embed.FS

// Handler returns an http.Handler for the built UI. Unknown paths fall back to
// index.html so client-side routing works on a hard refresh.
func Handler() http.Handler {
	dist, err := fs.Sub(assets, "dist")
	if err != nil {
		return http.NotFoundHandler()
	}
	if _, err := fs.Stat(dist, "index.html"); err != nil {
		// The binary was built without a UI bundle; the API still works.
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "web UI is not bundled in this build", http.StatusNotFound)
		})
	}

	files := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if clean == "" || clean == "." {
			serveIndex(w, r, dist)
			return
		}
		if _, err := fs.Stat(dist, clean); err != nil {
			serveIndex(w, r, dist)
			return
		}
		if strings.HasPrefix(clean, "assets/") {
			// Vite fingerprints asset filenames, so they are safe to cache hard.
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, dist fs.FS) {
	body, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(body)
}
