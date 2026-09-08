package httpserver

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// newStaticHandler serves files under root with directory-listing and
// dotfile access disabled, and defends against path traversal in addition
// to the protections already provided by the standard library.
func newStaticHandler(root string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean(r.URL.Path)
		if strings.Contains(clean, "..") {
			http.NotFound(w, r)
			return
		}
		if containsDotfileSegment(clean) {
			http.NotFound(w, r)
			return
		}

		full := filepath.Join(root, filepath.FromSlash(clean))
		// filepath.Join already cleans "..", but confirm the resolved path
		// still lives under root before touching the filesystem.
		if !strings.HasPrefix(full, filepath.Clean(root)+string(os.PathSeparator)) && full != filepath.Clean(root) {
			http.NotFound(w, r)
			return
		}

		info, err := os.Stat(full)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		if info.IsDir() {
			full = filepath.Join(full, "index.html")
			info, err = os.Stat(full)
			if err != nil || info.IsDir() {
				http.NotFound(w, r)
				return
			}
		}

		f, err := os.Open(full)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()

		w.Header().Set("Cache-Control", "public, max-age=300")
		http.ServeContent(w, r, full, info.ModTime(), f)
	})
}

func containsDotfileSegment(p string) bool {
	for _, part := range strings.Split(p, "/") {
		if strings.HasPrefix(part, ".") && part != "." {
			return true
		}
	}
	return false
}
