## 1. Data Model And Persistence

- [x] 1.1 Add an `ItemRating` model with item reference, rating value, optional device identifier, and timestamps in `server/models/models.go`
- [x] 1.2 Include the new rating model in database initialization so `AutoMigrate` creates the ratings table

## 2. API Changes

- [x] 2.1 Extend `/v1/device/{device_id}/next` responses to include `item_id` for both normal and placeholder items
- [x] 2.2 Add a rating handler and route for submitting a 1-5 rating for an item
- [x] 2.3 Validate rating requests for placeholder item ID, missing items, and out-of-range rating values before persisting

## 3. Verification

- [x] 3.1 Add handler and selector tests covering `/next` response shape changes and placeholder behavior
- [x] 3.2 Add API tests covering successful rating submission and invalid rating cases
- [x] 3.3 Run `go test ./...` and `go vet ./...`
