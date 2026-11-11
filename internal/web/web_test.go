package web

import (
	"context"
	"encoding/json"
	"github.com/m1ssha/scientific-mlops-core/catalog"
	"github.com/m1ssha/scientific-mlops-core/storage/local"
	"github.com/m1ssha/scientific-mlops-platform/internal/api"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssetsAndBoundary(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"index.html": "hello", "main.js": "script", "main.js.map": "secret", ".hidden.js": "secret"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		w.WriteHeader(418)
	})
	h, err := New(dir, "127.0.0.1:8080", api)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, method, host, origin string
		status                     int
	}{
		{"/", "GET", "127.0.0.1:8080", "", 200}, {"/main.js", "HEAD", "127.0.0.1:8080", "", 200}, {"/main.js.map", "GET", "127.0.0.1:8080", "", 404}, {"/.hidden.js", "GET", "127.0.0.1:8080", "", 404}, {"/missing", "GET", "127.0.0.1:8080", "", 404}, {"/api/v1/unknown", "GET", "127.0.0.1:8080", "", 418}, {"/api/v1/unknown", "HEAD", "127.0.0.1:8080", "", 405}, {"/", "POST", "127.0.0.1:8080", "", 405}, {"/", "GET", "evil", "", 403}, {"/", "GET", "127.0.0.1:8080", "http://evil", 403}, {"/../index.html", "GET", "127.0.0.1:8080", "", 404}} {
		r := httptest.NewRequest(tc.method, "http://127.0.0.1:8080"+tc.path, nil)
		r.Host = tc.host
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Errorf("%s %s: got %d want %d", tc.method, tc.path, w.Code, tc.status)
		}
		if w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Error("missing nosniff")
		}
		if tc.path == "/main.js" && !strings.HasPrefix(w.Header().Get("Content-Type"), "text/javascript") {
			t.Error("bad JS MIME")
		}
		if tc.method == "HEAD" && w.Body.Len() != 0 {
			t.Error("HEAD body")
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "http://127.0.0.1:8080/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Body.String() != "hello" {
		t.Error("not a snapshot")
	}
}
func TestBadAssets(t *testing.T) {
	d := t.TempDir()
	if _, e := New(d, "x", http.NotFoundHandler()); e == nil {
		t.Fatal("missing index")
	}
	os.WriteFile(filepath.Join(d, "index.html"), []byte("x"), 0600)
	os.Symlink("index.html", filepath.Join(d, "link.js"))
	if _, e := New(d, "x", http.NotFoundHandler()); e == nil {
		t.Fatal("symlink accepted")
	}
}

func TestAPIErrorContractWithWeb(t *testing.T) {
	root := t.TempDir()
	m, e := catalog.NewLocalMetadata(root)
	if e != nil {
		t.Fatal(e)
	}
	if e = m.InitRepository(context.Background(), "00000000000000000000000000000001"); e != nil {
		t.Fatal(e)
	}
	if _, e = local.New(root); e != nil {
		t.Fatal(e)
	}
	const authority = "127.0.0.1:8080"
	a, e := api.New(context.Background(), root, authority)
	if e != nil {
		t.Fatal(e)
	}
	assets := t.TempDir()
	if e = os.WriteFile(filepath.Join(assets, "index.html"), []byte("hello"), 0600); e != nil {
		t.Fatal(e)
	}
	h, e := New(assets, authority, a)
	if e != nil {
		t.Fatal(e)
	}
	for _, origin := range []string{"http://evil", "null"} {
		r := httptest.NewRequest("GET", "http://"+authority+"/api/v1/runs", nil)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var body map[string]map[string]string
		if w.Code != 403 || w.Header().Get("Content-Type") != "application/json" || json.Unmarshal(w.Body.Bytes(), &body) != nil || body["error"]["code"] != "forbidden" {
			t.Fatalf("API error contract: %d %v %s", w.Code, w.Header(), w.Body.String())
		}
	}
}

func TestHiddenSymlinkRejected(t *testing.T) {
	d := t.TempDir()
	if e := os.WriteFile(filepath.Join(d, "index.html"), []byte("ok"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink("index.html", filepath.Join(d, ".hidden")); e != nil {
		t.Fatal(e)
	}
	if _, e := New(d, "x", http.NotFoundHandler()); e == nil {
		t.Fatal("hidden symlink accepted")
	}
	if e := os.Remove(filepath.Join(d, ".hidden")); e != nil {
		t.Fatal(e)
	}
	if e := os.Mkdir(filepath.Join(d, ".hidden-dir"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink("../index.html", filepath.Join(d, ".hidden-dir", "link")); e != nil {
		t.Fatal(e)
	}
	if _, e := New(d, "x", http.NotFoundHandler()); e == nil {
		t.Fatal("symlink in hidden directory accepted")
	}
}
