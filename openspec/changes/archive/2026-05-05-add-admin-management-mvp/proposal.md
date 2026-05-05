## Why

The backend currently exposes only device-facing APIs, which makes it hard to inspect feed coverage, device activity, item consumption, and rating outcomes without querying the database directly. A lightweight internal admin surface is needed now to support day-to-day operations and content analysis while keeping the implementation simple and aligned with the existing Go service.

## What Changes

- Add an internal `/admin` management surface rendered by the existing Go server without authentication.
- Add dashboard, feed, device, and item views for browsing operational data and basic aggregate statistics.
- Add persistent item show tracking so item deliveries through `/v1/device/{device_id}/next` can be analyzed separately from NFC reads.
- Add persistent NFC read tracking so item reads can be analyzed separately from device polling and rating submission.
- Define read semantics explicitly: an item is considered "read" only when the NFC redirect endpoint is successfully used.
- Add backend pagination to feed, feed-item, item, and device admin pages to avoid rendering unbounded result sets.
- Add item list filtering by time range, feed, and title, plus sorting by reads, shows, ratings, and time.
- Clarify that ratings remain a distinct behavior from reads and do not imply a read event.

## Capabilities

### New Capabilities
- `admin-management`: Internal admin pages for viewing feeds, devices, items, and related aggregate metrics.
- `item-show-tracking`: Persistent tracking of item deliveries through the next-item API for later inspection in admin views.
- `item-read-tracking`: Persistent tracking of NFC-driven item reads for later inspection in admin views.

### Modified Capabilities
- `item-rating`: Clarify that rating records are separate from read records and are surfaced independently in admin reporting.

## Impact

- Affected code: `cmd/server/main.go`, `server/api/handlers.go`, `server/api/redirect.go`, database initialization and models, admin HTTP handlers/templates.
- Affected data: adds persistent item show and read records plus new aggregate queries across feeds, devices, items, shows, reads, and ratings.
- Affected behavior: introduces paginated/filterable admin routes under `/admin`, records item shows on `/v1/device/{device_id}/next`, and records NFC opens as read events.
