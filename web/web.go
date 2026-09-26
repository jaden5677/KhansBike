// Package web embeds the React front end so the whole site ships inside the
// single executable. `make web` builds the React app (frontend/) into
// web/dist before `go build`; without a build, only a placeholder is embedded
// and the API runs on its own.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Handler serves the embedded single-page app. Existing files are served as
// they are (fingerprinted files under assets/ are cached forever, dotfiles
// never); any other GET falls back to index.html so client-side routes such
// as /products/x survive a reload. With no build embedded it answers 404.
func Handler() http.Handler { return newHandler(dist, "dist") }

// newHandler serves the app found under dir in fsys.
func newHandler(fsys fs.FS, dir string) http.Handler {
	index, _ := fs.ReadFile(fsys, dir+"/index.html") // nil when no build is embedded

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" && name != "index.html" && !strings.HasPrefix(path.Base(name), ".") {
			file := dir + "/" + name
			if info, err := fs.Stat(fsys, file); err == nil && !info.IsDir() {
				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				http.ServeFileFS(w, r, fsys, file)
				return
			}
		}
		if index == nil {
			http.NotFound(w, r)
			return
		}
		// index.html must always be revalidated: it names the current assets.
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	})
}
