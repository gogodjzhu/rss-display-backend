## Context

The backend already tracks devices, feeds, and items in GORM-managed tables and serves the current display workflow through `/v1/device/{device_id}/next`. The current response omits the item ID, so clients have no stable identifier to send back later. The project also relies on `AutoMigrate`, so adding persistent rating data should be done by extending the Go model set rather than introducing a separate migration system.

## Goals / Non-Goals

**Goals:**
- Return the selected item ID from `/v1/device/{device_id}/next` for normal items.
- Add an API for submitting a 1-5 rating for an item.
- Persist ratings in the database with minimal schema and handler changes.
- Keep the current device flow simple and compatible with the existing no-framework HTTP structure.

**Non-Goals:**
- Using ratings to influence item selection in this change.
- Adding authentication, anti-abuse controls, or per-user identity beyond what the device flow already provides.
- Building reporting, aggregation, or analytics endpoints for ratings.

## Decisions

### Add a dedicated `ItemRating` model
Ratings will be stored in a new table managed by `AutoMigrate`, using a simple row-per-rating model. The row should include at least the rated `item_id`, the numeric rating value, and creation time. Including `device_id` is useful so ratings can be tied back to the device that submitted them without adding a user system. This keeps writes append-only and avoids complex update semantics.

Alternative considered: storing an aggregate score directly on `items`. This was rejected because it loses individual rating events and makes future analytics or deduplication harder.

### Expose `item_id` in `/next` responses
`/next` will return the selected item ID alongside title, image URL, and source. This is the minimal API change needed for clients to rate what they just displayed. Placeholder responses used during first-start sync should also return a stable ID value so the response shape stays consistent; the implementation can use `0` to mean "not rateable".

Alternative considered: requiring clients to infer the ID from `image_url`. This was rejected because it couples clients to URL parsing and breaks if the image path format changes.

### Add a write endpoint under `/v1/item/{item_id}/rating`
The rating API should accept a POST request with a small JSON payload containing the rating value and, optionally, the device ID if the path does not already include it. Using the item ID in the path keeps the endpoint explicit and reduces request ambiguity. The handler should validate that the item exists and the rating is in the 1-5 range before persisting.

Alternative considered: a query-string style GET or POST endpoint like `/rate?item_id=...&rating=...`. This was rejected because it is less clear, less REST-like, and harder to evolve.

### Keep validation strict and responses simple
Out-of-range ratings should return `400`. Unknown items should return `404`. Successful writes can return `204 No Content` or a small JSON confirmation; `204` is preferable because the write is straightforward and the client already knows what it sent.

## Risks / Trade-offs

- [Clients may try to rate the placeholder item] -> Treat placeholder ID `0` as invalid for persisted ratings and return `400`.
- [Multiple ratings from the same device for the same item may be submitted] -> Start with append-only storage and document that deduplication is out of scope for this change.
- [Changing `/next` response shape could affect clients that rigidly decode JSON] -> Only add a new field and keep existing fields unchanged.
- [Database growth from append-only ratings] -> Acceptable for current scale; the table is simple and can be aggregated later if needed.

## Migration Plan

1. Add the new rating model to `server/models/models.go` and `AutoMigrate`.
2. Extend `/next` response structs and tests to include `item_id`.
3. Add the new rating endpoint and validation.
4. Start the server normally; existing databases pick up the new table automatically through `AutoMigrate`.
5. Rollback, if needed, by deploying the previous binary; the extra table can remain unused.

## Open Questions

- Should the rating endpoint require `device_id` in the payload for every write, or should ratings be anonymous if the client cannot provide it?
- Should a repeated rating from the same device for the same item overwrite the prior rating in the future, or remain append-only permanently?
