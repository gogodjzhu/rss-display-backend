## 1. Data And Configuration

- [x] 1.1 Replace the item image persistence field from local image path to upstream image URL in `server/models/models.go`
- [x] 1.2 Add `rss.image_download_timeout_seconds` to config structs and `config.yaml`
- [x] 1.3 Update startup and runtime assumptions to remove dependence on the local processed image directory

## 2. RSS Polling Changes

- [x] 2.1 Update `server/rss/worker.go` so new items store only the extracted upstream image URL and publication time
- [x] 2.2 Remove pre-download, resize, local save, and backfill logic that exists only for eager image processing
- [x] 2.3 Keep and reuse the existing image URL extraction and text/color-card rendering helpers needed by request-time rendering

## 3. Request-Time Image Rendering

- [x] 3.1 Rewrite `server/image/handler.go` to load item metadata and render JPEG responses on demand
- [x] 3.2 Apply the configured download timeout to upstream image fetches, then resize and overlay title plus publication time on successful downloads
- [x] 3.3 Return generated color-card images when the item has no upstream image URL or request-time rendering fails

## 4. API And Observability

- [x] 4.1 Update `server/api/handlers.go` so every item response includes the backend-managed `/image/{item_id}.jpg` URL
- [x] 4.2 Add or update Prometheus metrics and logs for render attempts, download failures, and color-card fallbacks
- [x] 4.3 Run `go vet ./...` and manually verify `/next` plus `/image/{id}.jpg` behavior against the new flow
