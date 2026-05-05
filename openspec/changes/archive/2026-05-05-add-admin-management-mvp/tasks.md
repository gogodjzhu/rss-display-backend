## 1. Data Model And Read Tracking

- [x] 1.1 Add an `ItemRead` model and include it in database migration setup
- [x] 1.2 Update the NFC redirect flow to persist an item read record only on successful redirects
- [x] 1.3 Add or update tests covering successful and failed NFC read recording behavior

## 2. Admin Page Infrastructure

- [x] 2.1 Add admin route registration under `/admin` in the main HTTP server
- [x] 2.2 Add shared admin HTML template layout and basic styling for server-rendered pages
- [x] 2.3 Implement the admin dashboard handler with summary counts and recent activity sections

## 3. Feed And Item Admin Views

- [x] 3.1 Implement `/admin/feeds` with feed-level aggregate item, read, and rating metrics
- [x] 3.2 Implement `/admin/feeds/{id}` with feed metadata and item detail rows
- [x] 3.3 Implement `/admin/items` and `/admin/items/{id}` with related read and rating details

## 4. Device Admin Views

- [x] 4.1 Implement `/admin/devices` with device identifiers, last-seen timestamps, and aggregate read/rating counts
- [x] 4.2 Implement `/admin/devices/{device_id}` with separate read and rating record sections
- [x] 4.3 Add or update handler tests covering admin detail pages for existing and missing records

## 5. Verification

- [x] 5.1 Run `go test ./...` and fix any regressions caused by the admin MVP changes
- [x] 5.2 Run `go vet ./...` and address any issues introduced by the change

## 6. Show Tracking And List Controls

- [x] 6.1 Add an `ItemShow` model, migrate it, and persist show records for successful `/v1/device/{device_id}/next` responses
- [x] 6.2 Add or update tests covering successful and unsuccessful item show recording behavior
- [x] 6.3 Surface show counts and show detail records in the relevant admin pages
- [x] 6.4 Add backend pagination to feed list, feed detail item list, item list, and device list pages
- [x] 6.5 Add item list filtering by time range, feed, and title plus sorting by reads, shows, ratings, and time
- [x] 6.6 Run `go test ./...` and `go vet ./...` after the new admin changes
