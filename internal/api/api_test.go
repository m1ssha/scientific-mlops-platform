package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/m1ssha/scientific-mlops-core/catalog"
	"github.com/m1ssha/scientific-mlops-core/object"
	"github.com/m1ssha/scientific-mlops-core/storage/local"
)

type view struct {
	id      catalog.ID
	seq     int64
	records []catalog.Record
}

func (v *view) RepositoryID() catalog.ID { return v.id }
func (v *view) Sequence() int64          { return v.seq }
func (v *view) GetRecord(id catalog.ID) (catalog.Record, bool) {
	for _, r := range v.records {
		if r.ID == id {
			return r, true
		}
	}
	return catalog.Record{}, false
}
func (v *view) GetReference(string) (catalog.Reference, bool)     { return catalog.Reference{}, false }
func (v *view) GetOperation(catalog.ID) (catalog.Operation, bool) { return catalog.Operation{}, false }
func (v *view) ListRecords() []catalog.Record                     { return v.records }

type meta struct {
	v   *view
	err error
}

func (m *meta) Read(context.Context) (catalog.View, error)         { return m.v, m.err }
func (*meta) Update(context.Context, func(catalog.Tx) error) error { panic("write") }

type objects struct{}

func (objects) Verify(_ context.Context, r object.Ref) error {
	switch r.Digest[63:] {
	case "2":
		return object.ErrNotFound
	case "3":
		return object.ErrIntegrity
	case "4":
		return object.ErrStorage
	}
	return nil
}
func (objects) Open(context.Context, object.Ref) (io.ReadCloser, error)        { panic("open") }
func (objects) Put(context.Context, object.Ref, io.Reader) (object.Ref, error) { panic("put") }
func testServer() (*server, *meta) {
	id := func(n int) catalog.ID { return catalog.ID(fmt.Sprintf("%032x", n)) }
	ref := func(n int) object.Ref {
		return object.Ref{Algorithm: "sha256", Digest: fmt.Sprintf("%064x", n), Size: 1}
	}
	src := func(n int) catalog.MaterialSource {
		r := ref(n)
		return catalog.MaterialSource{Kind: "object", Object: &r}
	}
	snap := catalog.SnapshotBody{ProfileID: "historical", Inputs: []catalog.SnapshotInput{}, Parameters: map[string]catalog.TypedValue{}, Environment: catalog.Environment{Conditions: map[string]catalog.TypedValue{}}, Materials: []catalog.Material{{Name: "ok", Purpose: "code", Required: true, Path: "ok", Source: src(1)}, {Name: "missing", Purpose: "code", Required: true, Path: "missing", Source: src(2)}, {Name: "corrupt", Purpose: "code", Required: true, Path: "corrupt", Source: src(3)}, {Name: "io", Purpose: "code", Required: true, Path: "io", Source: src(4)}, {Name: "external", Purpose: "dependency", Required: true, Path: "external", Source: catalog.MaterialSource{Kind: "external", Locator: "https://example.invalid"}}}}
	run := catalog.RunBody{SnapshotID: id(1), Observations: map[string]catalog.Observation{}, Metrics: map[string]catalog.Metric{}, Outputs: []catalog.Output{}}
	m := &meta{v: &view{id: id(9), seq: 3, records: []catalog.Record{{ID: id(1), Kind: catalog.Snapshot, Sequence: 1, Body: snap}, {ID: id(2), Kind: catalog.Run, Sequence: 2, Body: run}, {ID: id(3), Kind: catalog.Run, Sequence: 3, Body: run}}}}
	b := boundStore{m, m.v.id}
	return &server{catalog.NewService(b, objects{}), b, "127.0.0.1:1234"}, m
}
func request(s http.Handler, method, path, host, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1:1234"+path, nil)
	r.Host = host
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func TestHTTPBoundaries(t *testing.T) {
	s, m := testServer()
	for _, tc := range []struct {
		method, path, host, origin string
		status                     int
	}{{"GET", "/api/v1/runs", "evil:1234", "", 403}, {"GET", "/api/v1/runs", "127.0.0.1:1234", "null", 403}, {"GET", "/api/v1/runs", "127.0.0.1:1234", "http://evil:1234", 403}, {"POST", "/api/v1/runs", "127.0.0.1:1234", "", 405}, {"GET", "/api/v1/runs?limit=2&limit=3", "127.0.0.1:1234", "", 400}, {"GET", "/api/v1/runs?after_sequence=1", "127.0.0.1:1234", "", 400}, {"GET", "/api/v1/runs?view_sequence=10", "127.0.0.1:1234", "", 400}, {"GET", "/api/v1/runs/not-valid", "127.0.0.1:1234", "", 400}, {"GET", "/api/v1/runs/000000000000000000000000000000ff", "127.0.0.1:1234", "", 404}} {
		w := request(s, tc.method, tc.path, tc.host, tc.origin)
		if w.Code != tc.status {
			t.Errorf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
		if w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("CORS enabled")
		}
	}
	m.v.id = "00000000000000000000000000000008"
	w := request(s, "GET", "/api/v1/runs", "127.0.0.1:1234", "")
	if w.Code != 409 {
		t.Fatal(w.Code)
	}
	if s.store.Update(context.Background(), nil) == nil {
		t.Fatal("write allowed")
	}
	m.err = catalog.ErrStorage
	w = request(s, "GET", "/api/v1/runs", "127.0.0.1:1234", "")
	if w.Code != 500 || !strings.Contains(w.Body.String(), "storage") {
		t.Fatal(w.Body.String())
	}
}
func TestPaginationAndRecovery(t *testing.T) {
	s, m := testServer()
	host := "127.0.0.1:1234"
	w := request(s, "GET", "/api/v1/runs?limit=1", host, "")
	var page catalog.Page
	if e := json.Unmarshal(w.Body.Bytes(), &page); e != nil || w.Code != 200 || len(page.Items) != 1 || page.NextAfterSequence == nil || page.ViewSequence != 3 {
		t.Fatal(w.Body.String(), e)
	}
	m.v.seq = 4
	m.v.records = append(m.v.records, catalog.Record{ID: "00000000000000000000000000000004", Kind: catalog.Run, Sequence: 4, Body: m.v.records[1].Body})
	w = request(s, "GET", fmt.Sprintf("/api/v1/runs?limit=1&after_sequence=%d&view_sequence=%d", *page.NextAfterSequence, page.ViewSequence), host, "")
	if e := json.Unmarshal(w.Body.Bytes(), &page); e != nil || len(page.Items) != 1 || page.Items[0].Sequence != 3 || page.NextAfterSequence != nil {
		t.Fatal(w.Body.String(), e)
	}
	w = request(s, "GET", "/api/v1/runs/00000000000000000000000000000002/recovery", host, "")
	var p catalog.RecoveryPlan
	if e := json.Unmarshal(w.Body.Bytes(), &p); e != nil || w.Code != 200 {
		t.Fatal(w.Body.String(), e)
	}
	want := []string{"verified", "missing", "corrupt", "io_error", "unchecked"}
	for i, x := range p.Materials {
		if x.Check != want[i] {
			t.Fatalf("%+v", p.Materials)
		}
	}
	if p.StoredMaterialsReady || p.RuntimeReady != "unchecked" || p.Execution != "unchecked" || p.NumericalAgreement != "unchecked" {
		t.Fatal(p)
	}
	w = request(s, "GET", "/api/v1/runs/compare?left=00000000000000000000000000000002&right=00000000000000000000000000000003", host, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "\"kind\":\"run\"") {
		t.Fatal(w.Body.String())
	}
}
func TestListenAndNoMutation(t *testing.T) {
	for _, a := range []string{"localhost:0", "0.0.0.0:0", "192.0.2.1:0"} {
		if l, e := Listen(a); e == nil {
			l.Close()
			t.Fatal(a)
		}
	}
	l, e := Listen("127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	root := t.TempDir()
	meta, e := catalog.NewLocalMetadata(root)
	if e != nil {
		t.Fatal(e)
	}
	if e = meta.InitRepository(context.Background(), "00000000000000000000000000000001"); e != nil {
		t.Fatal(e)
	}
	if _, e = local.New(root); e != nil {
		t.Fatal(e)
	}
	before := map[string]string{}
	snapshot := func() map[string]string {
		out := map[string]string{}
		e := filepath.Walk(root, func(path string, info os.FileInfo, e error) error {
			if e != nil {
				return e
			}
			if !info.IsDir() {
				b, e := os.ReadFile(path)
				if e != nil {
					return e
				}
				out[path] = string(b)
			}
			return nil
		})
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	before = snapshot()
	h, e := New(context.Background(), root, l.Addr().String())
	if e != nil {
		t.Fatal(e)
	}
	w := request(h, "GET", "/api/v1/runs", l.Addr().String(), "")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	after := snapshot()
	if fmt.Sprint(before) != fmt.Sprint(after) {
		t.Fatal("GET mutated store")
	}
	if _, e = New(context.Background(), filepath.Join(root, "missing"), l.Addr().String()); e == nil || errors.Is(e, os.ErrExist) {
		t.Fatal(e)
	}
}

func TestRecoveryReadsActualObjects(t *testing.T) {
	root := t.TempDir()
	store, e := local.New(root)
	if e != nil {
		t.Fatal(e)
	}
	data := []byte("valid content")
	sum := sha256.Sum256(data)
	ref := object.Ref{Algorithm: "sha256", Digest: hex.EncodeToString(sum[:]), Size: int64(len(data))}
	if _, e = store.Put(context.Background(), ref, bytes.NewReader(data)); e != nil {
		t.Fatal(e)
	}
	s, m := testServer()
	snap := m.v.records[0].Body.(catalog.SnapshotBody)
	snap.Materials = []catalog.Material{{Name: "input", Purpose: "code", Required: true, Path: "input", Source: catalog.MaterialSource{Kind: "object", Object: &ref}}}
	m.v.records[0].Body = snap
	s.service = catalog.NewService(s.store, store)
	check := func(want string) {
		t.Helper()
		w := request(s, "GET", "/api/v1/runs/00000000000000000000000000000002/recovery", "127.0.0.1:1234", "")
		var p catalog.RecoveryPlan
		if e := json.Unmarshal(w.Body.Bytes(), &p); e != nil || w.Code != 200 || len(p.Materials) != 1 || p.Materials[0].Check != want {
			t.Fatalf("want %s: %s %v", want, w.Body.String(), e)
		}
	}
	check("verified")
	path := filepath.Join(root, "objects", "sha256", ref.Digest)
	if e = os.WriteFile(path, []byte("tampered"), 0600); e != nil {
		t.Fatal(e)
	}
	check("corrupt")
	if e = os.Remove(path); e != nil {
		t.Fatal(e)
	}
	check("missing")
}
