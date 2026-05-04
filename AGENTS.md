# AGENTS.md

## Overview

Small Go backend for an ESP32 RSS display device. Fetches RSS feeds, downloads/resizes images to 320×240, stores in SQLite or MySQL, and serves a minimal REST API to devices.

- **Language:** Go 1.24.0
- **Module:** `github.com/esp32-rss-display/backend`
- **No framework** — stdlib `net/http` with Go 1.22+ pattern routing
- **ORM:** GORM with `AutoMigrate` (no migration files)
- **Dependencies:** vendored in `vendor/`

## Build

```bash
CGO_ENABLED=0 go build -o bin/server ./cmd/server
```

Run the server:
```bash
./bin/server
```

## No tests, no CI, no linter

- Zero `*_test.go` files exist
- No `.github/workflows/`, no Makefile, no Taskfile, no golangci config
- `go vet ./...` is the only available static check

## Key source layout

```
cmd/server/main.go          — entrypoint, route registration, worker startup
server/api/handlers.go      — GET /v1/device/{device_id}/next
server/api/redirect.go      — GET /nfc/{device_id}
server/image/handler.go     — GET /image/{id}.jpg
server/rss/worker.go        — RSS polling goroutine + image pipeline
server/database/database.go — GORM init + AutoMigrate (runs at startup)
server/models/models.go     — Device, Feed, Item structs
server/config/config.go     — YAML loader
server/metrics/metrics.go   — Prometheus counters
config.yaml                 — Runtime config (DB driver/DSN, port, feeds list)
```

## Configuration

All runtime config lives in `config.yaml` (not env vars). Key fields:

```yaml
database:
  driver: sqlite          # or "mysql"
  dsn: /tmp/data/rss.db
server:
  port: 8080
rss:
  fetch_interval_minutes: 15
  image_width: 320
  image_height: 240
feeds:
  - name: SomeFeed
    url: https://...
    enabled: true
```

To add/enable feeds, edit `config.yaml` — no DB seed step needed.

## Database

- SQLite default (pure-Go driver `glebarez/sqlite`, no CGo required)
- Schema created automatically via `AutoMigrate` on startup — no migration CLI
- Tables: `devices`, `feeds`, `items`
- Images stored on disk under `data/images/YYYYMMDD/<nanosecond>.jpg`
- **The parent directory of the SQLite DSN is created automatically on startup** if it doesn't exist.

## RSS image pipeline (7-stage fallback in `server/rss/worker.go`)

1. `<media:thumbnail>`
2. `<media:content medium="image">`
3. gofeed native `item.Image`
4. `<enclosure type="image/...">`
5. `<img src>` in HTML content/description
6. `<itunes:image href>`
7. Scrapes article page for og:image / twitter:image / itemprop / `<img>`

Images are resized (nearest-neighbor) and saved as JPEG quality 80.

## Devcontainer notes

- Image: `mcr.microsoft.com/devcontainers/go:1-1.24-bullseye`, user `root`, TZ `Asia/Shanghai`
- `init.sh` runs `go mod tidy && go mod vendor` on container creation
- Port 8080 forwarded automatically
- `opencode` CLI and `@fission-ai/openspec` are installed but `openspec/specs/` is empty — ignore
