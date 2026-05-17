# Tasks: Enforce Domain Layer

## Phase 1: Expand Domain Interfaces

### T1.1 — Refactor domain repository signatures to own `*gorm.DB`
- Move `*gorm.DB` from every method parameter to a struct field on `GORMRepository` and `serviceImpl`
- Update constructors: `NewGORMRepository(db *gorm.DB)` stores db
- Update all method signatures to drop `db *gorm.DB` / `db *gorm.DB` parameter
- Update callers in `cmd/server/main.go`, `devices/service_test.go`, `items/selector_test.go`, `feeds/service_test.go`

### T1.2 — Expand `domain/items` service
- Add `CreateIfNew(ctx, item *models.Item) (created bool, err error)` — tries to find by (feed_id, url), creates if not found
- Add `FindByIDFull(ctx, id uint) (*models.Item, error)` — returns full item (not just ID)
- Add `FindByTimeRange(ctx, start, end time.Time) ([]models.Item, error)` — for pipeline
- Add `UpdateContent(ctx, id uint, content string) error` — for crawl step
- Add `UpdateAbstract(ctx, id uint, abstract string) error` — for summarize step
- Add `ListEnabled(ctx) ([]models.Item, error)` — items from enabled feeds, ordered by time
- Implement in GORMRepository and serviceImpl

### T1.3 — Expand `domain/feeds` service
- Add `ListEnabled(ctx) ([]models.Feed, error)`
- Add `FindByID(ctx, id uint) (*models.Feed, error)`
- Implement in GORMRepository and serviceImpl

### T1.4 — Expand `domain/devices` service
- Add `UpdateCurrentItem(ctx, deviceID string, itemID uint) error`
- Add `TouchLastSeen(ctx, deviceID string) error`
- Ensure `GetOrCreate` and `UpdatePreference` work with DB-free signatures

### T1.5 — Create `domain/jobs` package
- Create `domain/jobs/repository.go` and `domain/jobs/service.go`
- Define `Repository` interface: `Create`, `FindByDeviceIDLatest`, `FindByIDAndDevice`, `UpdateDeviceID`, `UpdateReport`
- Define `Service` interface: `CreateJob`, `GetLatestJob`, `GetJobByID`, `AssociateDevice`, `UpdateReport`
- Implement GORMRepository with `*gorm.DB` field
- Absorb `database.GORMJobStore` logic into `domain/jobs`
- Ensure `pipeline.JobStore` interface is still satisfied (adapter or direct implementation)

### T1.6 — Create `domain/admin` package
- Create `domain/admin/read_model.go` with `ReadModelService` interface and struct view types (`FeedSummary`, `ItemSummary`, `DeviceSummary`, etc.)
- Create `domain/admin/repository.go` with GORM-backed aggregation queries
- Move all admin handler SQL logic into `ReadModelService` methods
- View types can live in `domain/admin` (they're presentation models, not domain models)

## Phase 2: Wire Dependency Injection

### T2.1 — Create DI container / wire in `cmd/server/main.go`
- Define handler structs that accept domain services via interfaces
- Construct all repositories with `db`, then services with repos
- Pass services to handlers, worker, pipeline

### T2.2 — Refactor `api/handlers.go` — `Handler` struct
- Create `api.Handler` struct with fields for `devices.Service`, `items.Service`, `items.ItemSelector`
- `GetNextItem` uses `deviceSvc.GetOrCreate` + `itemSelector.Select` + `itemSvc.RecordShow`
- `PostItemRating` uses `itemSvc.UpdateRating`

### T2.3 — Refactor `api/task_handlers.go`
- Add `jobs.Service` and `devices.Service` to handler struct
- `PutDevicePreference` uses `deviceSvc.UpdatePreference`
- `PostDeviceJob` uses `jobs.Service.CreateJob`
- `GetDeviceJob` / `GetDeviceJobByID` uses `jobs.Service`

### T2.4 — Refactor `api/redirect.go`
- `NFCRedirect` uses `deviceSvc.GetOrCreate` + `itemSvc.FindByIDFull` + `itemSvc.RecordRead`

### T2.5 — Refactor `admin/handler.go`
- `Handler` struct holds `admin.ReadModelService`
- All handler methods delegate to `ReadModelService` methods
- Remove all direct `database.GetDB()` and GORM calls from admin

### T2.6 — Refactor `image/handler.go`
- `Handler` struct holds `items.Finder` and `feeds.Finder` interfaces
- `ServeHTTP` uses domain services instead of raw GORM queries

### T2.7 — Refactor `rss/worker.go`
- `Worker` struct holds `feeds.Service` and `items.CreatorService`
- `fetchAllFeeds` uses `feeds.ListEnabled()`
- `fetchFeed` uses `items.CreateIfNew()` for each parsed item

## Phase 3: Refactor Pipeline Steps

### T3.1 — Define pipeline domain interfaces
- In `pipeline/steps/types.go`, define minimal interfaces:
  - `DeviceGetter`: `GetOrCreate(ctx, deviceID) → (*models.Device, error)`
  - `ItemRanger`: `FindByTimeRange(ctx, start, end) → ([]models.Item, error)`
  - `ItemUpdater`: `UpdateContent(ctx, id, content) error` + `UpdateAbstract(ctx, id, abstract) error`
  - `ItemFinder`: `FindByIDs(ctx, ids) → ([]models.Item, error)`
  - `JobReporter`: `UpdateReport(ctx, jobID, report, level2IDs) error`

### T3.2 — Refactor each pipeline step
- `filter_l1.go`: replace `*gorm.DB` with `DeviceGetter` + `ItemRanger`; remove `getDevice()` + `getItemsInRange()` helpers
- `crawl.go`: replace `*gorm.DB` with `ItemFinder` + `ItemUpdater`
- `summarize.go`: replace `*gorm.DB` with `ItemFinder` + `ItemUpdater`
- `filter_l2.go`: replace `*gorm.DB` with `DeviceGetter` + `ItemFinder`
- `compose.go`: replace `*gorm.DB` with `DeviceGetter` + `ItemFinder` + `JobReporter`

### T3.3 — Update `rss_pipeline.go` builder
- `BuildRSSPipeline` receives domain interfaces instead of `*gorm.DB`
- Wire appropriate domain service implementations

### T3.4 — Remove `domain/devices/job_service.go`
- Once `domain/jobs` is the canonical home for job operations, remove `devices.JobService` and `devices.JobStoreWithDeviceID`
- Update `api.InitRunner` / `api.Handler` wiring accordingly

## Phase 4: Cleanup

### T4.1 — Remove `database.GetDB()` global function
- Verify no callers remain outside `cmd/server/main.go` and `domain/` packages
- Delete `var DB *gorm.DB` and `func GetDB()` from `database/database.go`
- `database.Init()` returns `*gorm.DB` instead of setting a global

### T4.2 — Remove `database.GORMJobStore`
- Absorbed into `domain/jobs`; delete `database/job_store.go`

### T4.3 — Update tests
- Adapt `server/database/database_test.go` and `server/database/job_store_test.go` → move to `domain/jobs` tests
- Update `server/api/handlers_test.go`, `server/api/selector_test.go`, `server/api/handler_test.go`
- Update `server/admin/handler_test.go`
- Update `server/pipeline/pipeline_integration_test.go`

### T4.4 — Verify build and run `go vet ./...`
- Ensure no compile errors
- Remove unused imports of `database` and `gorm` from non-domain packages
- Run `go vet ./...`