# ADR-0005 — Standard-library HTTP, no web framework

**Status:** Accepted
**Date:** 2026-09-24

## Context

The server exposes a REST-ish JSON API plus serves a static dashboard. The Go ecosystem has
many routers/frameworks (chi, gin, echo, fiber). Do we need one?

## Decision

Use the standard library `net/http`. Since **Go 1.22**, the default `http.ServeMux` supports
method-aware pattern routing with path wildcards:

```go
mux.HandleFunc("GET /api/v1/prompts/{name}", h.getPrompt)
mux.HandleFunc("POST /api/v1/prompts/{name}/versions", h.createVersion)
```

That covers every route we need. Cross-cutting concerns (request logging, panic recovery,
JSON content-type, CORS for the dashboard) are handled by a small chain of `http.Handler`
middleware we write ourselves — a dozen lines each, and instructive to see.

## Rationale

- **Fewer dependencies, less magic.** A reader can follow every request from socket to handler
  without learning a framework's conventions.
- **Longevity.** `net/http` will be supported forever; a framework may not.
- **It's genuinely enough.** We don't need websockets, complex middleware ecosystems, or
  codegen. The 1.22 router closed the one real gap (method + path params) that used to justify
  reaching for chi.

## Consequences

- We write our own middleware (logging, recover, JSON helpers). This is a feature for a
  learning repo — the plumbing is visible, not hidden.
- Handlers receive `*http.Request` / `http.ResponseWriter` directly and use small helpers
  (`writeJSON`, `readJSON`, `writeError`) for consistency.
- If the API grew to dozens of routes with heavy shared middleware, revisiting this for `chi`
  would be reasonable — but that's not this project.
