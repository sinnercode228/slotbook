package httpapi

import (
	"net/http"
	"path"
	"path/filepath"
)

// staticFiles serves the built web app. Paths without a file extension are
// client-side routes and get index.html.
func staticFiles(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	index := filepath.Join(dir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", r.Method+" is not supported here")
			return
		}
		if path.Ext(r.URL.Path) == "" {
			http.ServeFile(w, r, index)
			return
		}
		files.ServeHTTP(w, r)
	})
}
