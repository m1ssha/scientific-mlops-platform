// Package web serves an immutable snapshot of a local production build.
package web

import (
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/m1ssha/scientific-mlops-platform/internal/api"
)

type asset struct {
	data        []byte
	contentType string
}

// New reads only regular, permitted build files. It never serves the store.
func New(dir, authority string, apiHandler http.Handler) (http.Handler, error) {
	root, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !root.IsDir() || root.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("web directory must be a directory")
	}
	assets := map[string]asset{}
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == dir {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("symlink in web directory")
		}
		for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
			if strings.HasPrefix(part, ".") {
				return nil
			}
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("nonregular web asset")
		}
		ext := strings.ToLower(filepath.Ext(rel))
		types := map[string]string{".html": "text/html; charset=utf-8", ".js": "text/javascript; charset=utf-8", ".css": "text/css; charset=utf-8", ".woff2": "font/woff2", ".svg": "image/svg+xml", ".png": "image/png", ".ico": "image/x-icon"}
		ct, ok := types[ext]
		if !ok || (ext == ".html" && rel != "index.html") {
			return nil
		}
		if ct == "" {
			ct = mime.TypeByExtension(ext)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		assets["/"+filepath.ToSlash(rel)] = asset{data, ct}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(assets["/index.html"].data) == 0 {
		return nil, errors.New("missing or empty index.html")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" {
			apiHandler.ServeHTTP(w, r)
			return
		}
		if !api.Allowed(r, authority) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}
		a, ok := assets[path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", a.contentType)
		w.Header().Set("Content-Length", strconv.Itoa(len(a.data)))
		if r.Method == http.MethodGet {
			_, _ = w.Write(a.data)
		}
	}), nil
}
