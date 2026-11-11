// Package api exposes the local, read-only core prototype.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/m1ssha/scientific-mlops-core/catalog"
	"github.com/m1ssha/scientific-mlops-core/storage/local"
)

var errRepositoryMismatch = errors.New("repository changed")

type boundStore struct {
	catalog.MetadataStore
	id catalog.ID
}

func (b boundStore) Read(ctx context.Context) (catalog.View, error) {
	v, e := b.MetadataStore.Read(ctx)
	if e != nil {
		return nil, e
	}
	if v.RepositoryID() != b.id {
		return nil, errRepositoryMismatch
	}
	return v, nil
}
func (b boundStore) Update(context.Context, func(catalog.Tx) error) error {
	return errors.New("read-only metadata")
}

type server struct {
	service   *catalog.Service
	store     boundStore
	authority string
}

// New opens an existing store and pins its repository identity.
func New(ctx context.Context, root, authority string) (http.Handler, error) {
	meta, e := catalog.NewLocalMetadata(root)
	if e != nil {
		return nil, e
	}
	objects, e := local.OpenReadOnly(root)
	if e != nil {
		return nil, e
	}
	v, e := meta.Read(ctx)
	if e != nil {
		return nil, e
	}
	b := boundStore{meta, v.RepositoryID()}
	return &server{catalog.NewService(b, objects), b, authority}, nil
}

// Listen only accepts a literal loopback IP; callers must use the resulting authority.
func Listen(address string) (net.Listener, error) {
	host, _, e := net.SplitHostPort(address)
	if e != nil {
		return nil, e
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return nil, errors.New("listen address must be a literal loopback IP")
	}
	return net.Listen("tcp", address)
}

// Allowed applies the same authority boundary to API and static resources.
func Allowed(r *http.Request, authority string) bool {
	if r.Host != authority || len(r.Header.Values("Origin")) > 1 {
		return false
	}
	if _, ok := r.Header["Origin"]; ok && r.Header.Get("Origin") == "" {
		return false
	}
	return r.Header.Get("Origin") == "" || r.Header.Get("Origin") == "http://"+authority
}
func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if !Allowed(r, s.authority) {
		fail(w, 403, "forbidden")
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		fail(w, 405, "method_not_allowed")
		return
	}
	q, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil {
		fail(w, 400, "invalid_input")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	var result any
	switch {
	case r.URL.Path == "/api/v1/repository":
		if !params(q) {
			fail(w, 400, "invalid_input")
			return
		}
		v, err := s.store.Read(ctx)
		e = err
		if e == nil {
			result = struct {
				RepositoryID catalog.ID `json:"repository_id"`
				Sequence     int64      `json:"sequence"`
			}{v.RepositoryID(), v.Sequence()}
		}
	case r.URL.Path == "/api/v1/runs":
		if !params(q, "limit", "after_sequence", "view_sequence") {
			fail(w, 400, "invalid_input")
			return
		}
		var limit, after, view int64
		limit, e = number(q, "limit", 50)
		if e == nil {
			after, e = number(q, "after_sequence", 0)
		}
		if e == nil {
			view, e = number(q, "view_sequence", -1)
		}
		if e != nil || limit <= 0 || limit > 1000 || after < 0 || view < -1 || (after > 0 && view == -1) {
			fail(w, 400, "invalid_input")
			return
		}
		if view == -1 {
			var v catalog.View
			v, e = s.store.Read(ctx)
			if e == nil {
				view = v.Sequence()
			}
		}
		if e == nil {
			result, e = s.service.ListRuns(ctx, catalog.ListOptions{Limit: limit, AfterSequence: after, ViewSequence: view})
		}
	case r.URL.Path == "/api/v1/runs/compare":
		if !params(q, "left", "right") || q.Get("left") == "" || q.Get("right") == "" {
			fail(w, 400, "invalid_input")
			return
		}
		result, e = s.service.CompareRuns(ctx, catalog.ID(q.Get("left")), catalog.ID(q.Get("right")))
	case strings.HasPrefix(r.URL.Path, "/api/v1/runs/"):
		if !params(q) {
			fail(w, 400, "invalid_input")
			return
		}
		tail := strings.TrimPrefix(r.URL.Path, "/api/v1/runs/")
		parts := strings.Split(tail, "/")
		if len(parts) == 1 && parts[0] != "" {
			result, e = s.service.GetRun(ctx, catalog.ID(parts[0]))
		} else if len(parts) == 2 && parts[0] != "" && parts[1] == "recovery" {
			result, e = s.service.PlanRecovery(ctx, catalog.ID(parts[0]))
		} else {
			fail(w, 404, "not_found")
			return
		}
	default:
		fail(w, 404, "not_found")
		return
	}
	if e != nil {
		log.Printf("API error: %v", e)
		status, code := classify(e)
		fail(w, status, code)
		return
	}
	if e = json.NewEncoder(w).Encode(result); e != nil {
		log.Printf("encode response: %v", e)
	}
}
func params(q url.Values, allowed ...string) bool {
	set := map[string]bool{}
	for _, k := range allowed {
		set[k] = true
	}
	for k, v := range q {
		if !set[k] || len(v) != 1 || v[0] == "" {
			return false
		}
	}
	return true
}
func number(q url.Values, key string, def int64) (int64, error) {
	v, ok := q[key]
	if !ok {
		return def, nil
	}
	if strings.Trim(v[0], "0123456789") != "" {
		return 0, errors.New("invalid number")
	}
	return strconv.ParseInt(v[0], 10, 64)
}
func classify(e error) (int, string) {
	switch {
	case errors.Is(e, catalog.ErrInvalidInput):
		return 400, "invalid_input"
	case errors.Is(e, catalog.ErrNotFound):
		return 404, "not_found"
	case errors.Is(e, errRepositoryMismatch):
		return 409, "repository_mismatch"
	case errors.Is(e, catalog.ErrNotInitialized):
		return 500, "not_initialized"
	case errors.Is(e, catalog.ErrIntegrity):
		return 500, "integrity"
	case errors.Is(e, catalog.ErrUnsupportedSchema):
		return 500, "unsupported_schema"
	case errors.Is(e, catalog.ErrStorage):
		return 500, "storage"
	case errors.Is(e, context.DeadlineExceeded):
		return 504, "deadline"
	case errors.Is(e, context.Canceled):
		return 408, "cancelled"
	default:
		return 500, "internal"
	}
}
func fail(w http.ResponseWriter, status int, code string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": fmt.Sprintf("request failed: %s", code)}})
}
