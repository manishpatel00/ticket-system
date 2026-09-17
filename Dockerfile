# --- Build stage ---
# Pinned Go version, Alpine base for a small, fast build environment.
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy go.mod first so Docker can cache the (empty) dependency layer
# independently of source changes. This project has zero external
# dependencies by design, so there is no go.sum to copy — `go mod download`
# is a no-op, but the pattern is kept for correctness if deps are ever added.
COPY go.mod ./
RUN go mod download

COPY . .

# CGO_ENABLED=0 produces a fully static binary so the final image can be
# `scratch` with no libc at all. Build flags strip debug symbols (-s -w)
# to shrink the binary.
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /ticket-system .

# --- Runtime stage ---
# distroless-style minimal image: just CA certs (for any future outbound
# HTTPS calls) and the static binary. No shell, no package manager —
# smaller attack surface than a full alpine runtime image.
FROM alpine:3.20

RUN apk add --no-cache ca-certificates && \
    addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app
COPY --from=builder /ticket-system /app/ticket-system

USER appuser
EXPOSE 8080

# JWT_SECRET has no default on purpose (see main.go: the app refuses to
# start without one). Set it via `docker run -e JWT_SECRET=...` or the
# deployment platform's environment variable settings.
ENV PORT=8080

ENTRYPOINT ["/app/ticket-system"]
