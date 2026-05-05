## Context

The current backend is a single Go HTTP service with device-facing JSON endpoints, SQLite/MySQL persistence through GORM, and no existing frontend or template layer. Operational inspection currently requires reading logs or querying the database directly. The MVP admin scope is intentionally narrow: internal-only pages without authentication, simple device metadata, no large-scale analytics concerns, and a strict semantic distinction between "read" and "rated" behavior.

The current data model supports feeds, items, devices, and item ratings. Device polling updates `devices.current_item_id` and `devices.last_seen`, and NFC redirects resolve the current item URL, but the system does not persist any show history for `/next` deliveries or any read history for NFC opens. This means the admin experience cannot currently answer questions like which items a device has been shown, which items it has read, how many shows a feed has, or whether an item has only been delivered but never opened.

## Goals / Non-Goals

**Goals:**
- Add a lightweight `/admin` surface inside the existing Go service.
- Support browsing feeds, devices, and items with basic aggregate metrics.
- Persist `/next` deliveries as show records so "show" has a precise and queryable meaning.
- Persist NFC opens as read records so "read" has a precise and queryable meaning.
- Keep the implementation simple enough for SQLite and the current no-framework backend.
- Define read and rating semantics clearly in specs so future analytics do not blur the two behaviors.
- Add simple backend pagination to admin list/detail pages that can grow unbounded.
- Add basic item list filtering and sorting without introducing a separate admin API.

**Non-Goals:**
- Authentication or authorization for admin routes.
- A separate frontend application, API-first admin backend, or SPA architecture.
- Rich charting, background aggregation jobs, or high-volume analytics optimizations.
- Additional device metadata such as labels, locations, or groups.
- A generalized event stream covering every possible user action.

## Decisions

### Use server-rendered HTML routes under `/admin`
The admin UI will be delivered by the same Go HTTP server using `html/template` and route handlers such as `/admin`, `/admin/feeds`, `/admin/feeds/{id}`, `/admin/devices`, `/admin/devices/{device_id}`, `/admin/items`, and `/admin/items/{id}`.

Rationale:
- The repository has no frontend build pipeline, static assets, or template system today.
- The MVP is table- and detail-page oriented, which fits server-side rendering well.
- Keeping everything in one binary minimizes implementation and maintenance cost.

Alternatives considered:
- Separate SPA frontend: rejected because it adds build tooling, API surface area, and coordination overhead for a small internal tool.
- JSON-only admin API with no HTML: rejected because the immediate need is operable pages, not another integration surface.

### Record reads in a dedicated `item_reads` table
The system will add a new persistent table for item reads with fields for item identity, device identity, and creation time. A read record is created only when `GET /nfc/{device_id}` successfully resolves the device's current item and issues the redirect.

Rationale:
- The product definition says "read" means NFC-opened, not merely served to the device.
- A dedicated table keeps the data model simple and directly queryable for feed/device/item reporting.
- For MVP, a purpose-built table is easier to reason about than a generalized event model.

Alternatives considered:
- Reusing `devices.current_item_id` plus timestamps: rejected because it only stores current state and loses history.
- Introducing a generalized item event table: rejected for MVP because current requirements only need read and rating behaviors, which are clearer as separate persisted concepts.

### Record shows in a dedicated `item_shows` table
The system will add a new persistent table for item shows with fields for item identity, device identity, and creation time. A show record is created only when `GET /v1/device/{device_id}/next` successfully selects a persisted item and updates device state.

Rationale:
- The user wants explicit delivery tracking that is separate from NFC reads.
- A dedicated table keeps the semantics obvious: shows come from `/next`, reads come from `/nfc`, ratings come from `/rating`.
- This supports admin reporting of show-to-read behavior without reinterpreting `devices.current_item_id`.

Alternatives considered:
- Inferring shows from `devices.current_item_id` changes: rejected because it only stores the latest item and loses history.
- Merging shows and reads into a single event table: rejected to keep the MVP semantics straightforward and query logic simple.

### Keep read and rating reporting separate, then combine at the page level
Admin pages will surface read counts and rating counts side by side, but ratings will not imply reads and reads will not imply ratings. Device detail pages will present separate read and rating record sections rather than a merged "activity" abstraction.

Rationale:
- The user explicitly wants the semantics kept separate.
- Separate sections reduce ambiguity in reporting and avoid accidental product drift.
- Query logic remains straightforward because reads come from `item_reads` and ratings from `item_ratings`.

Alternatives considered:
- Treating ratings as a subtype of reads: rejected because a rating may exist without a successful NFC read in the persisted history.
- Presenting a unified event timeline only: rejected for MVP because it obscures the semantic distinction that the spec needs to preserve.

### Prefer direct aggregate queries over precomputed summaries
The admin pages will compute counts, averages, and recent records from the normalized tables at request time, using straightforward SQL/GORM queries.

Rationale:
- The requested scope explicitly does not optimize for large-scale data volume.
- Avoiding summary tables or background jobs keeps the design small and reduces migration complexity.

Alternatives considered:
- Materialized summary tables: rejected as premature for MVP.
- Cached analytics jobs: rejected because freshness and operational simplicity matter more than scale here.

### Apply pagination and item list controls in the server-rendered admin handlers
The admin handlers will accept query parameters for page navigation, and the item list handler will also accept feed, title, time-range, and sort parameters. Because the requested scope does not optimize for large data volume, the implementation may enrich item summaries first and then apply filtering, sorting, and pagination in-process before rendering the page.

Rationale:
- This keeps the admin surface simple and avoids introducing a separate query API.
- The user explicitly does not need large-scale optimization yet.
- Sorting by aggregate fields such as reads, shows, and ratings is easier after enrichment.

Alternatives considered:
- Fully SQL-driven filtering and sorting across all aggregate dimensions: rejected as unnecessary complexity for the current scope.
- Frontend-side filtering and pagination: rejected because the requirement is backend pagination and the project has no frontend application.

## Risks / Trade-offs

- [NFC redirects become responsible for persistence] -> Keep read insertion minimal and only write after all redirect prerequisites succeed, so failed lookups do not create false reads.
- [`/next` becomes responsible for show persistence] -> Record shows only after a real item is selected and device state is saved, so placeholder responses and failed updates do not create false shows.
- [Admin pages may issue multiple aggregate queries] -> Accept this for MVP and keep pages modest; revisit with summary tables only if real usage shows a problem.
- [Filtering and sorting are initially applied after enrichment] -> Accept this while data volume is small and reevaluate if list latency becomes noticeable.
- [No admin authentication] -> Limit this to internal/non-public deployments as agreed and keep the design explicit that this is not for internet exposure.
- [Read counts represent open attempts that successfully reached redirect, not user dwell or article completion] -> Define this clearly in specs and UI labels as "reads" or "NFC opens" based on page context.
- [Show counts represent server deliveries, not confirmed device display completion] -> Define this clearly in specs and UI labels as "shows" or "served" based on page context.
