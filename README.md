# Ticket System — Backend Intern Assignment

## Submission

| Item                        | Link                                                                 |
|-----------------------------|----------------------------------------------------------------------|
| **GitHub Repository**       | https://github.com/manishpatel00/ticket-system                       |
| **Deployed Application URL**| https://ticket-system-it9n.onrender.com                              |
| **Public Health Check URL** | https://ticket-system-it9n.onrender.com/health                       |

---

A small REST API where a user can register, log in, create tickets, list
and view only their own tickets, and move a ticket through a fixed status
workflow (`open → in_progress → closed`, with `closed` as a terminal
state). Built in Go, authenticated with JWT, containerized with Docker.

## Design choices & assumptions

- **Zero external dependencies.** Routing uses Go 1.22's standard-library
  `net/http.ServeMux` (method + path-pattern routing, e.g.
  `mux.HandleFunc("GET /tickets/{id}", ...)`), and both JWT signing/
  verification and password hashing are implemented directly on top of
  `crypto/hmac`, `crypto/sha256`, `crypto/rand`, and `crypto/subtle`
  (HS256 for JWTs, PBKDF2-HMAC-SHA256 with a random per-user salt and
  100,000 iterations for passwords, following RFC 2898). This was a
  deliberate simplicity trade-off given the "keep it simple, do not
  over-engineer" note in the brief: no `go.sum`, no supply-chain surface,
  and a Docker build that never needs to reach a module proxy.
- **Storage:** in-memory, guarded by a single `sync.RWMutex` (allowed by
  the brief). See `internal/store/store.go`. It is safe under Go's default
  one-goroutine-per-request server. Swapping in Postgres/SQLite later only
  requires a new implementation of the `store.Store` interface — no
  handler code would change.
- **Ownership checks return 404, not 403.** If you request or update a
  ticket you don't own, the API returns `404 Not Found` rather than
  `403 Forbidden`, so a user can't distinguish "doesn't exist" from
  "exists but isn't yours." This is a common, deliberate anti-enumeration
  choice.
- **Status transitions are strictly forward, one step at a time:**
  `open → in_progress → closed`. Skipping a step (`open → closed`),
  moving backward, or touching a closed ticket at all returns
  `409 Conflict`.
- **Login errors are uniform.** "No such user" and "wrong password" both
  return the same `401` with the same message, to avoid leaking which
  emails are registered.
- **Password minimum length:** 8 characters (not specified in the brief;
  a reasonable default).
- **IDs** are random 16-character hex strings (`crypto/rand`), not
  sequential integers, so they aren't guessable/enumerable.

## Project layout

```
.
├── main.go                        # wiring: config, router, graceful shutdown
├── main_test.go                   # end-to-end HTTP tests against the real router
├── internal/
│   ├── auth/
│   │   ├── jwt.go                 # HS256 JWT issue/verify (stdlib only)
│   │   ├── password.go            # PBKDF2 password hashing (stdlib only)
│   │   ├── jwt_test.go
│   │   └── password_test.go
│   ├── models/
│   │   ├── models.go               # User, Ticket, status state machine
│   │   └── models_test.go
│   ├── store/
│   │   ├── store.go                # Store interface + in-memory implementation
│   │   └── store_test.go           # includes a concurrency test
│   ├── handlers/
│   │   ├── health.go
│   │   ├── auth_handler.go         # /auth/register, /auth/login
│   │   ├── ticket_handler.go       # /tickets ...
│   │   └── respond.go              # shared JSON response helpers
│   └── middleware/
│       ├── auth.go                 # JWT bearer-token middleware
│       └── logging.go              # request logging
├── Dockerfile
├── .env.example
└── .gitignore
```

## API reference

All responses are JSON. Errors are always `{"error": "message"}`.

| Method | Path                     | Auth? | Purpose                        |
|--------|--------------------------|-------|---------------------------------|
| GET    | `/health`                | No    | Health check                   |
| POST   | `/auth/register`         | No    | Register a user                |
| POST   | `/auth/login`            | No    | Log in, get a JWT               |
| POST   | `/tickets`               | Yes   | Create a ticket                |
| GET    | `/tickets`               | Yes   | List your own tickets          |
| GET    | `/tickets/{id}`          | Yes   | Get one of your own tickets    |
| PATCH  | `/tickets/{id}/status`   | Yes   | Update status of your ticket   |

Protected routes require `Authorization: Bearer <token>`.

### `GET /health`
```
curl http://localhost:8080/health
→ 200 {"status":"ok"}
```

### `POST /auth/register`
```
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"alice@example.com","password":"password123"}'
→ 201 {"id":"5406ce06ad8f6fd2","email":"alice@example.com"}
```
`400` invalid email / password too short · `409` email already registered

### `POST /auth/login`
```
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"alice@example.com","password":"password123"}'
→ 200 {"token":"eyJhbGciOi..."}
```
`401` invalid email or password

### `POST /tickets`
```
curl -X POST http://localhost:8080/tickets \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"title":"Cannot login","description":"Getting a 500 on login"}'
→ 201 {"id":"...","user_id":"...","title":"Cannot login","description":"...","status":"open","created_at":"...","updated_at":"..."}
```
`400` missing title · `401` missing/invalid token

### `GET /tickets`
Returns only the caller's own tickets, as a JSON array (`[]` if none).

### `GET /tickets/{id}`
`200` the ticket · `404` doesn't exist or isn't yours

### `PATCH /tickets/{id}/status`
```
curl -X PATCH http://localhost:8080/tickets/$ID/status \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"status":"in_progress"}'
→ 200 { ...ticket with updated status and updated_at }
```
`400` unknown status value · `404` doesn't exist or isn't yours ·
`409` transition not allowed from the ticket's current status

## Running locally (no Docker)

Requires Go 1.22+.

```bash
export JWT_SECRET=some-long-random-string   # required, app refuses to start without it
go run .
# in another terminal:
curl http://localhost:8080/health
```

## Running the tests

```bash
go test ./... -v
```
24 tests across `auth`, `models`, `store`, and full end-to-end HTTP tests
in `main_test.go` (registration, login, ticket CRUD, ownership isolation
between two users, the full status state machine including every invalid
transition, and unauthenticated-request rejection). `internal/store` also
has a concurrency test (`go test -race ./...` if you have a C toolchain
available for the race detector).

## Local Run Contract (Docker)

```bash
docker build -t ticket-system .
docker run -p 8080:8080 -e JWT_SECRET=some-long-random-string ticket-system
curl http://localhost:8080/health
```
Expected response:
```json
{"status": "ok"}
```

The Dockerfile is a two-stage build: `golang:1.22-alpine` compiles a
static binary (`CGO_ENABLED=0`), then a minimal `alpine:3.20` runtime
image (running as a non-root user) just copies that binary in. No
`go.sum` is needed since the project has no external dependencies, which
keeps the image build deterministic and independent of module-proxy
availability.

## Deployment (free tier)

Any platform that can run a Dockerfile and lets you set environment
variables works. [Render](https://render.com) is a straightforward
option with a free web-service tier that builds directly from a
Dockerfile:

1. Push this repository to GitHub.
2. On Render: **New → Web Service** → connect the repo.
3. Render should auto-detect the `Dockerfile`; if prompted, choose
   **Docker** as the environment (no build/start command needed — the
   `Dockerfile`'s `ENTRYPOINT` handles that).
4. Under **Environment**, add `JWT_SECRET` with a long random value.
   (`PORT` doesn't need to be set — Render provides its own `PORT`, but
   this app defaults to `8080` and the Dockerfile also sets `ENV
   PORT=8080`; if Render injects a different `PORT` value it will still
   be picked up since the app reads `os.Getenv("PORT")`.)
5. Deploy. The live public URL is
   `https://ticket-system-it9n.onrender.com`; `/health` on that URL is
   the public health-check URL.

Other free-tier options that work the same way (push a Dockerfile,
connect a repo, set `JWT_SECRET`): [Fly.io](https://fly.io) and
[Railway](https://railway.app). Free-tier terms (spin-down behavior,
credit-card requirements, monthly hour caps) change over time on all of
these platforms, so check current terms on whichever you pick before
relying on it.

**Note on free-tier cold starts:** several free platforms spin a service
down after a period of inactivity and take ~30–60 seconds to wake back up
on the next request. That's a platform characteristic, not a bug in this
service — if a health check or the hidden test suite times out on first
request, retrying once after a short wait should succeed.

## Submission checklist

- [x] GitHub repository link — https://github.com/manishpatel00/ticket-system
- [x] Deployed application URL — https://ticket-system-it9n.onrender.com
- [x] Public `/health` URL — https://ticket-system-it9n.onrender.com/health
- [x] This README (local run, Docker run, deployment URL, assumptions)
- [x] `.env.example`
