# Design: RSS Content Pipeline

## Architecture Overview

The pipeline is a Go-orchestrated, Python-executed system. Go manages the database, task state, and pipeline sequencing. Python handles all LLM and crawling operations. Communication is via JSON files on disk.

```
┌───────────────────────────────────────────────────────────────┐
│  Go Backend                                                   │
│                                                               │
│  POST /v1/device/{device_id}/task                            │
│       │                                                       │
│       ▼                                                       │
│  ┌──────────┐    ┌─────────────────────────────────────────┐ │
│  │ Create   │───▶│ goroutine: runPipeline(taskID)         │ │
│  │ Task row │    │                                       │ │
│  └──────────┘    │ For each stage:                       │ │
│                  │   1. Build input JSON from DB          │ │
│  ←── 201 + task  │   2. exec.Command(python3 pipeline.py)│ │
│                  │   3. Read output JSON                  │ │
│                  │   4. Update DB (task + items)          │ │
│                  └─────────────────────────────────────────┘ │
│                                                               │
│  GET /v1/device/{device_id}/task[/{task_id}]                 │
│       └──▶ Query task status + report                        │
│                                                               │
└───────────────────────────────────────────────────────────────┘
         │
         │ 5 × exec.Command, file-based I/O
         ▼
┌───────────────────────────────────────────────────────────────┐
│  Python (pipeline.py)                                         │
│                                                               │
│  --mode filter_l1   --input path --output path              │
│  --mode crawl       --input path --output path              │
│  --mode summarize   --input path --output path              │
│  --mode filter_l2   --input path --output path              │
│  --mode compose     --input path --output path              │
│                                                               │
│  Internal modules:                                            │
│  ┌────────────┐  ┌────────────┐  ┌──────────────────┐       │
│  │ crawl4ai   │  │ LangChain  │  │ rate_limiter.py   │       │
│  │ (crawl)    │  │ (4 LLM    │  │ per-domain delay  │       │
│  │            │  │  modes)   │  │ 30-90s random     │       │
│  └────────────┘  └────────────┘  └──────────────────┘       │
└───────────────────────────────────────────────────────────────┘
```

## Data Model Changes

### Device (extend existing)

Add two columns:

| Column | Type | Description |
|--------|------|-------------|
| `role` | `text` | Short role description, e.g. "后端开发者" |
| `preference` | `text` | Free-text preference, e.g. "关注分布式系统、Go语言、AI应用" |

### Item (extend existing)

Add two columns:

| Column | Type | Description |
|--------|------|-------------|
| `content` | `longtext` | Full article content crawled and converted to Markdown |
| `abstract` | `longtext` | LLM-generated summary (~200 chars) |

### Task (new table)

| Column | Type | Description |
|--------|------|-------------|
| `id` | `uint` PK | Auto-increment |
| `device_id` | `string` | FK → devices.device_id |
| `status` | `string` | One of: `pending`, `filtering_l1`, `crawling`, `summarizing`, `filtering_l2`, `composing`, `completed`, `failed` |
| `time_range_start` | `*time.Time` | Query start time |
| `time_range_end` | `*time.Time` | Query end time |
| `level1_ids` | `text` | JSON array of item IDs after L1 filter |
| `level2_ids` | `text` | JSON array of item IDs after L2 filter |
| `report` | `longtext` | Final Markdown report |
| `error` | `text` | Error message on failure |
| `created_at` | `time.Time` | |
| `updated_at` | `time.Time` | |
| `completed_at` | `*time.Time` | |

## Pipeline Stages

### Stage 1: filter_l1

**Go input JSON:**
```json
{
  "device": { "role": "后端开发者", "preference": "关注分布式系统..." },
  "items": [
    { "id": 1, "title": "...", "url": "..." },
    { "id": 5, "title": "...", "url": "..." }
  ]
}
```

**Python output JSON:**
```json
{
  "level1_ids": [1, 5, 33]
}
```

**Go action:** Update `task.level1_ids`, set `task.status = "crawling"`.

### Stage 2: crawl

**Go input JSON:**
```json
{
  "items": [
    { "id": 1, "url": "https://..." },
    { "id": 5, "url": "https://..." }
  ]
}
```

**Python output JSON:**
```json
{
  "results": [
    { "id": 1, "content": "# Article Title\n...", "success": true },
    { "id": 5, "content": "", "success": false, "error": "timeout" }
  ]
}
```

**Go action:** For each successful result, update `Item.content`. Set `task.status = "summarizing"`.

**Rate limiting:** Each URL's domain is tracked. Before crawling, sleep for `(30..90 random seconds) - time_since_last_access_for_domain`. Minimum sleep is 0. State persisted in `data/pipeline/rate_limits.json`.

### Stage 3: summarize

**Go input JSON:**
```json
{
  "items": [
    { "id": 1, "title": "...", "content": "md..." },
    { "id": 5, "title": "...", "content": "md..." }
  ]
}
```

**Python output JSON:**
```json
{
  "results": [
    { "id": 1, "abstract": "..." },
    { "id": 5, "abstract": "..." }
  ]
}
```

**Go action:** Update `Item.abstract` for each. Set `task.status = "filtering_l2"`.

### Stage 4: filter_l2

**Go input JSON:**
```json
{
  "device": { "role": "...", "preference": "..." },
  "items": [
    { "id": 1, "title": "...", "abstract": "..." },
    { "id": 5, "title": "...", "abstract": "..." }
  ]
}
```

**Python output JSON:**
```json
{
  "level2_ids": [1, 5]
}
```

**Go action:** Update `task.level2_ids`, set `task.status = "composing"`.

### Stage 5: compose

**Go input JSON:**
```json
{
  "device": { "role": "...", "preference": "..." },
  "items": [
    { "id": 1, "title": "...", "abstract": "...", "url": "..." },
    { "id": 5, "title": "...", "abstract": "...", "url": "..." }
  ]
}
```

**Python output JSON:**
```json
{
  "report": "## 2026-05-15 技术精选\n\n### 分布式系统\n- **[标题](url)**: 摘要...\n\n---\n*基于您的偏好自动整理*"
}
```

**Go action:** Update `task.report`, set `task.status = "completed"`, set `task.completed_at`.

## Concurrency Control

Only one task may run at a time globally. This is enforced via:

- A `sync.Mutex` guarding a `runningTaskID` pointer in Go.
- On new task creation: if `runningTaskID != nil`, reject with `409 Conflict`.
- On task completion/failure: clear `runningTaskID`.

## File Layout

### New Go files

```
server/pipeline/pipeline.go    — Pipeline orchestrator (runPipeline, stage logic)
server/pipeline/python.go      — exec.Command wrapper, file I/O helpers
server/api/task_handlers.go     — HTTP handlers for task + preference APIs
server/models/models.go        — Extended Device, Item; new Task
```

### New Python files

```
py/pipeline.py                 — Unified entry point with --mode flag
py/rate_limiter.py             — Per-domain rate limiter (persistent JSON state)
```

### Data directory

```
data/pipeline/
  {task_id}_filter_l1_in.json     — Input files (preserved for debugging)
  {task_id}_filter_l1_out.json    — Output files (preserved for debugging)
  {task_id}_crawl_in.json
  {task_id}_crawl_out.json
  {task_id}_summarize_in.json
  {task_id}_summarize_out.json
  {task_id}_filter_l2_in.json
  {task_id}_filter_l2_out.json
  {task_id}_compose_in.json
  {task_id}_compose_out.json
  rate_limits.json                  — Domain last-access timestamps
```

All intermediate files are preserved (never deleted) to facilitate debugging.

## Config Addition

```yaml
pipeline:
  python_path: "python3"
  script_path: "py/pipeline.py"
  data_dir: "data/pipeline"
  rate_limit_min_seconds: 30
  rate_limit_max_seconds: 90
```

## API Endpoints

### PUT /v1/device/{device_id}/preference

```json
Request:
{
  "role": "后端开发者",
  "preference": "关注分布式系统、Go语言、AI应用、硬件创业"
}

Response: 200 OK
{
  "device_id": "esp32-001",
  "role": "后端开发者",
  "preference": "关注分布式系统、Go语言、AI应用、硬件创业"
}
```

### POST /v1/device/{device_id}/task

```json
Request:
{
  "time_range_start": "2026-05-01T00:00:00Z",
  "time_range_end": "2026-05-15T00:00:00Z"
}

Response: 201Created
{
  "id": 1,
  "device_id": "esp32-001",
  "status": "pending",
  "time_range_start": "2026-05-01T00:00:00Z",
  "time_range_end": "2026-05-15T00:00:00Z",
  "created_at": "2026-05-15T10:00:00Z"
}

Error (task already running): 409 Conflict
```

### GET /v1/device/{device_id}/task

Returns the latest task for the device.

### GET /v1/device/{device_id}/task/{task_id}

Returns the full task including report when completed.

## Error Handling

- Any stage that fails sets `task.status = "failed"` and `task.error` to the Python stderr output or Go error message.
- Crawl failures for individual items are non-fatal — the item's `success: false` is recorded and that item is skipped in subsequent stages.
- If a task is already running, new task requests return `409 Conflict`.