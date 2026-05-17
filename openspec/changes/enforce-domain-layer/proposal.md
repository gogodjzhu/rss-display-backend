# Proposal: Enforce Domain Layer

## Problem

The codebase has a `server/domain/` layer with repositories and services for `devices`, `feeds`, and `items`, but virtually every module bypasses it. Handlers in `api/`, `admin/`, `image/`, `rss/`, and `pipeline/steps/` all call `database.GetDB()` directly and perform raw GORM operations on `models.*` structs.

This means:
- Business logic is scattered across handlers (e.g. device get-or-create is re-implemented in `api/handlers.go`, `api/task_handlers.go`, and `pipeline/steps/filter_l1.go`)
- No single source of truth for data access patterns
- Impossible to swap persistence strategy (e.g. caching, different DB) without touching every handler
- Testing requires a real database; no seam for mocks at the domain boundary

## Goal

Make `server/domain/` the **only** path to database operations. All modules outside `domain/` must go through domain services and must not import `database` or `gorm` directly.

## Scope

### In Scope
- Expand `domain/devices`, `domain/feeds`, `domain/items` services to cover all needed operations
- Add `domain/jobs` service (for Job CRUD currently in task_handlers and compose step)
- Refactor `api/`, `admin/`, `image/`, `rss/`, `pipeline/steps/` to use domain services
- Remove `database.GetDB()` calls from all non-domain, non-main code
- Pass `*gorm.DB` as a dependency injection rather than a global

### Out of Scope
- Schema changes or new database tables
- Changing the `models` package structure
- Admin dashboard UI changes
- Pipeline Python scripts
- Changing `server/database/init.go` (startup wiring is fine to touch)

## Approach

Incremental, one module at a time, always keeping the build green. Start with the read-only consumers (admin, image) then move to write-path consumers (api handlers, rss worker, pipeline steps).