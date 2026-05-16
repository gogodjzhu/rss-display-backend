# Tasks: Add RSS Pipeline

## Phase 1: Data Model & Configuration

### Task 1.1: Extend Device and Item models
- [x] Add `Role string` and `Preference string` fields to `Device` struct in `server/models/models.go`
- [x] Add `Content string` and `Abstract string` fields to `Item` struct
- [x] Both new text fields use `type:text` / `type:longtext` GORM tags

### Task 1.2: Create Task model
- [x] Create `Task` struct in `server/models/models.go` with fields per design
- [x] `Task.TableName()` returns `"tasks"`
- [x] Add `Task` to AutoMigrate in `server/database/database.go`

### Task 1.3: Add pipeline config
- [x] Add `PipelineConfig` struct to `server/config/config.go`
- [x] Add `Pipeline PipelineConfig` field to `Config` struct
- [x] Add defaults and yaml tags
- [x] Update `config.yaml` with pipeline section

## Phase 2: Go Orchestrator

### Task 2.1: Create pipeline package — python.go
- [x] Create `server/pipeline/python.go`
- [x] Implement `Run(mode, inputPath, outputPath string) error` — exec.Command wrapper
- [x] Implement `WriteJSON(path, data)` and `ReadJSON(path, target)`
- [x] Create data dir on startup if needed
- [x] Log command, stdout, stderr for debugging

### Task 2.2: Create pipeline package — pipeline.go
- [x] Create `server/pipeline/pipeline.go`
- [x] Implement `runPipeline(taskID uint)` — sequential 5-stage orchestrator
- [x] Each stage: build input → write JSON → exec python → read JSON → update DB
- [x] Stage-specific input builders and result appliers
- [x] On any stage failure: set task.status="failed", task.error=stderr, return
- [x] On completion: set task.status="completed", task.completed_at=now

### Task 2.3: Concurrency guard
- [x] Add `sync.Mutex` + `runningTaskID *uint` to pipeline package
- [x] `StartPipeline(deviceID string, timeStart, timeEnd time.Time) (*models.Task, error)`
- [x] Goroutine: defer clear runningTaskID, call RunPipeline

### Task 2.4: Create pipeline data dir logic
- [x] On pipeline start, ensure `cfg.Pipeline.DataDir` exists
- [x] File naming: `{taskID}_{stage}_in.json` and `{taskID}_{stage}_out.json`
- [x] Preserve all files (never delete)

## Phase 3: API Handlers

### Task 3.1: PUT /v1/device/{device_id}/preference
- [x] Create `server/api/task_handlers.go`
- [x] Parse role + preference from request body
- [x] Upsert into Device row
- [x] Return updated device info

### Task 3.2: POST /v1/device/{device_id}/task
- [x] Parse `time_range_start` and `time_range_end` from request body (RFC3339)
- [x] Validate device exists (or auto-create)
- [x] Call `pipeline.StartPipeline(deviceID, start, end)`
- [x] Return 201 with task JSON on success
- [x] Return 409 if a task is already running

### Task 3.3: GET /v1/device/{device_id}/task
- [x] Query most recent task for device
- [x] Return task JSON including status and report

### Task 3.4: GET /v1/device/{device_id}/task/{task_id}
- [x] Query specific task by ID
- [x] Return full task JSON

### Task 3.5: Register routes in main.go
- [x] Add all new routes to mux in `cmd/server/main.go`
- [x] Wire up pipeline config to pipeline package

## Phase 4: Python Pipeline

### Task 4.1: Create pipeline.py unified entry point
- [x] Create `py/pipeline.py`
- [x] Parse args: `--mode` (filter_l1|crawl|summarize|filter_l2|compose), `--input`, `--output`
- [x] Read input JSON, dispatch to mode handler, write output JSON

### Task 4.2: Implement filter_l1 mode
- [x] Use LangChain with ChatOpenAI to filter items relevant to user preference
- [x] Return `{"level1_ids": [...]}`

### Task 4.3: Implement crawl mode
- [x] Apply rate limiter (30–90s delay between requests to same domain)
- [x] Use crawl4ai `AsyncWebCrawler` to fetch each URL
- [x] Convert to Markdown, return results with success/error per item

### Task 4.4: Implement summarize mode
- [x] Use LangChain to generate ~200-char abstract per item
- [x] Return `{"results": [{"id": ..., "abstract": "..."}]}`

### Task 4.5: Implement filter_l2 mode
- [x] Use LangChain to select most relevant items given the preference and abstracts
- [x] Return `{"level2_ids": [...]}`

### Task 4.6: Implement compose mode
- [x] Use LangChain to generate a structured Markdown report
- [x] Return `{"report": "## ..."}`

### Task 4.7: Create rate_limiter.py
- [x] Create `py/rate_limiter.py`
- [x] Persistent state in `data/pipeline/rate_limits.json`
- [x] `acquire(domain)` method: check last access time → sleep if needed → update timestamp
- [x] Random delay between min/max seconds (configurable via input)

## Phase 5: Integration & Testing

### Task 5.1: Wire pipeline config through main.go
- [x] Pass `cfg.Pipeline` to pipeline package initialization
- [x] Ensure data dir creation on startup
- [x] Register pipeline routes

### Task 5.2: Manual integration test
- [x] Set up DASHSCOPE_API_KEY env var
- [x] Set device preference via API
- [x] Trigger task via API with a small time range
- [x] Monitor task status progression
- [x] Verify item content/abstract fields populated
- [x] Verify task report generated