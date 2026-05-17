# Design: Enforce Domain Layer

## Architecture

```
BEFORE (current)                          AFTER (target)
━━━━━━━━━━━━━━━━━━━                       ━━━━━━━━━━━━━━━━━━━

  api/handlers ──→ database.GetDB()          api/handlers ──→ domain/devices
  api/task_handlers ──→ database.GetDB()     api/task_handlers ──→ domain/devices
                                                               ──→ domain/jobs
  api/redirect ──→ database.GetDB()          api/redirect ──→ domain/devices
                                               └─→ domain/items

  admin/handler ──→ database.GetDB()         admin/handler ──→ domain/devices
                                                 └─→ domain/feeds
                                                 └─→ domain/items
                                                 └─→ domain/admin (new: read aggregations)

  image/handler ──→ database.GetDB()         image/handler ──→ domain/items
                                                 └─→ domain/feeds

  rss/worker ──→ database.GetDB()            rss/worker ──→ domain/feeds
                                                 └─→ domain/items

  pipeline/steps ──→ *gorm.DB (injected)    pipeline/steps ──→ domain/devices
                                                 └─→ domain/items
                                                 └─→ domain/jobs

  cmd/server/main.go                        cmd/server/main.go
    └─→ database.Init()                       └─→ database.Init()
                                               └─→ wire domain services with db
```

## Key Design Decisions

### D1: Dependency injection replaces global `database.GetDB()`

Services receive `*gorm.DB` at construction time (via a `DB` field), not via `database.GetDB()` calls scattered through business logic. `cmd/server/main.go` is the composition root that wires everything.

Before:
```go
// scattered in every handler
db := database.GetDB()
db.Where("device_id = ?", id).First(&device)
```

After:
```go
// cmd/server/main.go
db := database.InitDB(cfg)
deviceRepo := devices.NewGORMRepository()
deviceSvc := devices.NewService(deviceRepo, db)

// api handler receives deviceSvc
handler := api.NewHandler(deviceSvc, itemsSvc, ...)
```

### D2: Repositories own `*gorm.DB`, handlers don't

Each domain repository receives `*gorm.DB` at creation and stores it. Service methods take `context.Context` but not `*gorm.DB`. This removes `*gorm.DB` from all handler signatures.

Before:
```go
func (s *serviceImpl) GetOrCreate(ctx context.Context, db *gorm.DB, deviceID string) (*models.Device, error)
```

After:
```go
func (s *serviceImpl) GetOrCreate(ctx context.Context, deviceID string) (*models.Device, error)
```

### D3: `domain/admin` for complex aggregation queries

Admin dashboard has complex multi-table JOINs and aggregation queries that don't fit neatly into the existing entity-centric domain services. These go into a new `domain/admin` package with a `ReadModel` service that provides pre-computed summary structures.

```go
type ReadModelService interface {
    DashboardSummary(ctx context.Context) (*DashboardSummary, error)
    ListFeeds(ctx context.Context, page, pageSize int) (*FeedListView, error)
    FeedDetail(ctx context.Context, feedID uint) (*FeedDetailView, error)
    ListItems(ctx context.Context, filters ItemFilters, page, pageSize int) (*ItemListView, error)
    ItemDetail(ctx context.Context, itemID uint) (*ItemDetailView, error)
    ListDevices(ctx context.Context, page, pageSize int) (*DeviceListView, error)
    DeviceDetail(ctx context.Context, deviceID string) (*DeviceDetailView, error)
}
```

### D4: `domain/jobs` for Job CRUD

Job-related operations are currently split between `database/job_store.go` (implements `pipeline.JobStore`) and direct GORM calls in `api/task_handlers.go` and `pipeline/steps/compose.go`. Create a `domain/jobs` service that covers all Job operations:

```go
type Service interface {
    CreateJob(ctx context.Context, deviceID string, timeStart, timeEnd time.Time) (*models.Job, error)
    GetLatestJob(ctx context.Context, deviceID string) (*models.Job, error)
    GetJobByID(ctx context.Context, jobID uint, deviceID string) (*models.Job, error)
    UpdateJobReport(ctx context.Context, jobID uint, report string, level2IDs []uint) error
}
```

This absorbs `database.GORMJobStore` and `devices.JobService` into a single coherent package. `pipeline.JobStore` becomes an interface satisfied by the new domain.

### D5: Pipeline steps receive domain interfaces, not `*gorm.DB`

Currently:
```go
func NewFilterL1Step(db *gorm.DB, runner *pipeline.PythonRunner) *FilterL1Step
```

After:
```go
type DeviceQuerier interface {
    GetOrCreate(ctx context.Context, deviceID string) (*models.Device, error)
}
type ItemQuerier interface {
    ListByTimeRange(ctx context.Context, start, end time.Time) ([]models.Item, error)
}
func NewFilterL1Step(devices DeviceQuerier, items ItemQuerier, runner *pipeline.PythonRunner) *FilterL1Step
```

This makes pipeline steps testable without a database.

### D6: `rss/worker` uses domain services

The RSS worker currently queries feeds and creates items directly. After refactoring:
```go
type Worker struct {
    feedSvc   feeds.Service
    itemSvc   items.CreatorService  // new: CreateIfNew(item) → bool
    extractor *items.ImageExtractor
}
```

### D7: Domain services that need new methods

**`domain/items`** needs:
- `CreateIfNew(ctx, item) → (bool, error)` — used by rss/worker
- `FindByIDFull(ctx, id) → (*models.Item, error)` — used by image handler
- `FindByTimeRange(ctx, start, end) → ([]models.Item, error)` — used by pipeline
- `UpdateContent(ctx, id, content) → error` — used by pipeline crawl step
- `UpdateAbstract(ctx, id, abstract) → error` — used by pipeline summarize step
- `Select(ctx, device) → (*models.Item, error)` — already exists in selector

**`domain/devices`** needs:
- `GetOrCreate(ctx, deviceID) → (*models.Device, error)` — already exists but needs DI
- `UpdateCurrentItem(ctx, deviceID, itemID) → error`
- `TouchLastSeen(ctx, deviceID) → error`

**`domain/feeds`** needs:
- `ListEnabled(ctx) → ([]models.Feed, error)` — for rss/worker
- `FindByID(ctx, id) → (*models.Feed, error)` — for image handler

## Migration Strategy

Phase order ensures each phase keeps the build green:

1. **Expand domain interfaces** — Add missing methods to existing domain services and new `domain/admin`, `domain/jobs` packages. No breaking changes.
2. **Wire DI in main.go** — Construct domain services with DB and pass to handlers. Handlers still call `database.GetDB()` alongside.
3. **Migrate consumers one module at a time** — api → admin → image → rss worker → pipeline steps. Each module stops importing `database`/`gorm`.
4. **Remove global DB** — Once no non-domain code calls `database.GetDB()`, remove the global. `cmd/server/main.go` passes DB to domain services only.