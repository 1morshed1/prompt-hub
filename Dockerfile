# Multi-stage build for the PromptHub server. Build context is the REPO ROOT so
# both the Go module (server/) and the static dashboard (web/) are available.
#
#   docker build -t prompthub .
#
# Produces a self-contained image: a static binary that serves the API and the
# dashboard, with SQL migrations embedded in the binary.

# ---- build stage ----
FROM golang:1.24-alpine AS build
WORKDIR /src

# Cache module downloads separately from source for faster rebuilds.
COPY server/go.mod server/go.sum ./
RUN go mod download

COPY server/ ./
# CGO off -> a fully static binary that runs on a minimal image.
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/prompthub ./cmd/server

# ---- run stage ----
FROM alpine:3.20
RUN adduser -D -u 10001 app
WORKDIR /app

COPY --from=build /out/prompthub /app/prompthub
COPY web /app/web

ENV PORT=8080
ENV WEB_DIR=/app/web
EXPOSE 8080
USER app

ENTRYPOINT ["/app/prompthub"]
