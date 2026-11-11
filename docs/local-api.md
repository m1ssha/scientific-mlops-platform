# Local read-only HTTP API

This is a reversible prototype under the proposed ADRs, for one owner and one existing core repository. It is not a remotely exposed service, authentication mechanism, or multi-user boundary. Start it with:

```sh
go run ./cmd/platform --store /path/to/existing/store --listen 127.0.0.1:8080
```

The listener accepts only a literal loopback IP. Port 0 is supported; the actual address is logged. Requests must use the exact listener authority in Host. If Origin is present, it must equal `http://` plus that authority; `null`, other origins, and multiple Origin headers are rejected. No CORS headers are sent. API accepts only GET. When the optional web build is enabled, static resources also accept HEAD; both use the same Host and Origin boundary. A local process may access the service; these checks do not authenticate it. The store must already exist and be initialized. Its repository ID is pinned at startup and checked on every metadata view; a changed identity returns `repository_mismatch`. GET does not initialize or modify the store. Concurrent modification by the trusted store owner is outside the security boundary.

All API responses are JSON with `Cache-Control: no-store`:

- `GET /api/v1/repository` returns `repository_id` and current `sequence`.
- `GET /api/v1/runs?limit=50&after_sequence=0&view_sequence=N` returns the core Page. `limit` is 1–1000 (default 50). On the first request omit `view_sequence` to capture the current view. For the next page pass both the returned `view_sequence` and `next_after_sequence` as `after_sequence`. A positive `after_sequence` requires an explicit view. A view beyond the current sequence is invalid. This fixes the visible sequence while new records are appended.
- `GET /api/v1/runs/{id}` returns a core run Record.
- `GET /api/v1/runs/compare?left=ID&right=ID` returns the core run Diff. It does not judge numerical agreement.
- `GET /api/v1/runs/{id}/recovery` returns the core RecoveryPlan. It verifies all declared local objects by reading their bytes, so the request may be slow. Cancellation or the 30-second deadline interrupts verification. A successful response does not mean the experiment ran or results agree; `runtime_ready`, `execution`, and `numerical_agreement` remain `unchecked`. Missing, corrupt, unchecked, and I/O material checks remain in the plan. The core plan's diagnostics and manifest values may contain local paths and locators.

Unknown, duplicated, empty or malformed query parameters are rejected. Errors have `{"error":{"code":"...","message":"..."}}` with generic client-facing messages: `invalid_input` (400), `forbidden` (403), `not_found` (404), `method_not_allowed` (405 with Allow), `repository_mismatch` (409), `cancelled` (408), `deadline` (504), or `not_initialized`, `integrity`, `unsupported_schema`, `storage`, `internal` (500). Detailed errors are logged locally. A 200 recovery response can contain blockers and is not a claim of readiness. No endpoint exposes object bytes, executes a pipeline, or modifies records. Core is pinned to a specific commit; no adjacent checkout is required.

An optional same-origin interface is described in [local-ui.md](local-ui.md).
