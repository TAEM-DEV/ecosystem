# TAEM — Ecosystem
## taem-dev/ecosystem Repository Brief for Claude Code

Read this completely before writing any code.

---

## What This Repo Is

`taem-dev/ecosystem` is the persistent semantic layer for TAEM.
It is a standalone service backed by Qdrant. It is infrastructure —
not mission state, not application code.

It maintains five Qdrant collections that represent the live state of the
ecosystem, the history of all missions, the ADR constraint corpus, learned
operational knowledge, and validated wiring patterns.

"Don't mix rocket fuel with coffee." — mission artifacts (signals.jsonl,
step-plan.json) live in mc-state. Semantic representations and learned
knowledge live here. They are different things with different lifecycles.

---

## ADR-006 Hard Constraints — Read Before Touching Anything

**C-006-001 (HARD):** The five collections are: repo_surfaces,
mission_memory, constraint_index, wiring_patterns, lessons_learned.
Never create collections named signals, manifest, step_plan, or remediation.
Raw mission records are mc-state's domain. Never here.

**C-006-002 (HARD):** Write ownership per collection:
- repo_surfaces → NAV only
- mission_memory → kernel.mission_close only
- constraint_index → ARCH only
- wiring_patterns → kernel.mission_close only
- lessons_learned → kernel.mission_close only
The service enforces this at the API layer.

**C-006-003 (HARD):** NAV cache check: always diff last_commit_sha before
re-reading a repo. Unchanged repos must use cached surface.

**C-006-004 (HARD):** If ecosystem is unreachable, NAV emits HOLD.
This service must handle graceful degradation — return 503 clearly so
the kernel can surface HOLD correctly.

**C-006-005 (SOFT):** Entries older than 24 hours → stale flag.
Entries older than 7 days → must re-read regardless of SHA match.

---

## Repo Layout

```
taem-dev/ecosystem/
├── service/
│   ├── main.go                    # HTTP service wrapping Qdrant
│   └── handlers/
│       ├── repo_surfaces.go       # GET/PUT /api/repo_surfaces/{repo}
│       ├── mission_memory.go      # POST /api/mission_memory
│       ├── constraint_index.go    # GET /api/constraints/search
│       ├── wiring_patterns.go     # GET/POST /api/wiring_patterns
│       └── lessons_learned.go     # GET/POST /api/lessons
├── collections/
│   └── init.go                    # Creates all 5 Qdrant collections on startup
├── schemas/
│   ├── repo_surface.schema.json
│   ├── mission_memory.schema.json
│   ├── constraint.schema.json
│   ├── wiring_pattern.schema.json
│   └── lesson.schema.json
├── Dockerfile
├── docker-compose.yml             # ecosystem service + Qdrant
└── CLAUDE.md                      # this file
```

---

## Service API

The ecosystem exposes a simple HTTP API that the kernel calls via
`internal/ecosystem/client.go`. These are the endpoints:

```
GET  /health                              → 200 OK or 503
GET  /api/repo_surfaces/{org}/{repo}      → repo surface or 404
PUT  /api/repo_surfaces/{org}/{repo}      → write (NAV only, enforced by writer header)
GET  /api/repo_surfaces/{org}/{repo}/stale → bool — is cache stale?

POST /api/mission_memory                  → write mission summary (kernel only)
GET  /api/mission_memory/search?q={task}  → semantic search, top-5 similar missions

GET  /api/constraints/search?q={query}   → semantic search over constraint_index
POST /api/constraints/index              → write/update (ARCH only)

GET  /api/wiring_patterns/search?from={}&to={}&via={} → find validated patterns
POST /api/wiring_patterns                → write validated pattern (kernel only)

GET  /api/lessons/search?q={task}        → semantic search, top-10 relevant lessons
POST /api/lessons                        → write lesson (kernel only)
```

---

## Write Authorization

Every write endpoint checks the `X-TAEM-Writer` header against the
allowed writers from ADR-006 C-006-002. Return 403 if the writer is
not authorized for that collection.

```go
var collectionWriters = map[string][]string{
    "repo_surfaces":    {"NAV"},
    "mission_memory":   {"kernel.mission_close"},
    "constraint_index": {"ARCH"},
    "wiring_patterns":  {"kernel.mission_close"},
    "lessons_learned":  {"kernel.mission_close"},
}
```

---

## Qdrant Collection Setup

All five collections use 1536-dimensional vectors (OpenAI Ada embedding size
— use the same dimension for future compatibility even if using a different
embedding model locally).

For local development without an embedding model, store the raw text in a
payload field and use Qdrant's keyword filter as a fallback until embeddings
are available. Never block on embedding availability — the service should
work with keyword search if vector search isn't configured.

```go
// collections/init.go
// CreateAll() creates all 5 collections if they don't exist.
// Safe to call on every startup — idempotent.
collections := []string{
    "taem_repo_surfaces",
    "taem_mission_memory",
    "taem_constraint_index",
    "taem_wiring_patterns",
    "taem_lessons_learned",
}
```

---

## Environment Variables

| Variable | Required | Description |
|---|---|---|
| `QDRANT_URL` | Yes | Qdrant service URL e.g. http://localhost:6333 |
| `QDRANT_API_KEY` | Optional | If Qdrant is authenticated |
| `EMBEDDING_URL` | Optional | Embedding service URL — if absent, use keyword fallback |
| `PORT` | Optional | HTTP port (default 8765) |
| `FRESHNESS_HOURS` | Optional | Stale threshold (default 24) |
| `FORCE_STALE_HOURS` | Optional | Force re-read threshold (default 168 = 7 days) |

---

## docker-compose.yml

```yaml
services:
  qdrant:
    image: qdrant/qdrant:latest
    ports: ["6333:6333"]
    volumes: ["qdrant_data:/qdrant/storage"]

  ecosystem:
    build: .
    ports: ["8765:8765"]
    environment:
      QDRANT_URL: http://qdrant:6333
    depends_on: [qdrant]

volumes:
  qdrant_data:
```

---

## Build Order

1. `collections/init.go` — Qdrant collection creation, idempotent
2. Schemas in `schemas/` — one JSON Schema per collection entry type
3. `service/handlers/repo_surfaces.go` — most critical, NAV depends on it
4. `service/handlers/lessons_learned.go` — PRB-Skeptic depends on this
5. `service/handlers/constraint_index.go` — ARCH and PRB-ADR depend on this
6. Remaining handlers
7. `service/main.go` — wire it all together
8. `Dockerfile` + `docker-compose.yml`
9. Smoke test: start service, call /health, write a repo surface, read it back

---

## Hard Rules

- This service is stateless beyond Qdrant — no in-memory caches that diverge
  from Qdrant state
- Write authorization (C-006-002) must be enforced at the HTTP handler layer,
  not just documented
- Return 503 clearly when Qdrant is unavailable — the kernel maps 503 to HOLD
- Never accept raw mission JSONL data — if a caller tries to write signals or
  manifests here, return 400 with "use mc-state for mission records"
- The freshness staleness policy (C-006-005) runs as a background goroutine
  that marks entries stale — it does not delete them
