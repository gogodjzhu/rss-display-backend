## Why

The device currently can only fetch the next item to display, but it cannot send feedback about whether the content was useful. Adding item ratings allows the backend to capture user preference signals and ties them to specific items, which also requires `/v1/device/{device_id}/next` to return the `item_id` needed for later feedback submission.

## What Changes

- Add a new rating API that accepts an `item_id` and a rating value from 1 to 5.
- Persist rating records in the database so ratings survive restarts and can be used later for analytics or recommendation work.
- Extend `/v1/device/{device_id}/next` responses to include the selected item's ID.
- Validate rating input and return clear errors for unknown items or out-of-range rating values.

## Capabilities

### New Capabilities
- `item-rating`: Allow clients to receive item IDs from `/next` and submit 1-5 ratings for displayed items that are stored by the backend.

### Modified Capabilities

## Impact

- Affects `server/api/handlers.go` and route registration in `cmd/server/main.go`.
- Requires a new persisted model and `AutoMigrate` update in `server/models/models.go` and `server/database/database.go`.
- Adds new API tests for `/next` and the rating endpoint.
- Introduces a new REST write path that clients can call after showing content.
