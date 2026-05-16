## Requirements

### Requirement: Devices can store user preference information
The system SHALL allow each device to store a role description and a free-text preference string that describe the user's interests. These preferences are used by the pipeline to filter and personalize content.

#### Scenario: Setting device preference
- **WHEN** a client sends `PUT /v1/device/{device_id}/preference` with a JSON body containing `role` and/or `preference`
- **THEN** the system SHALL update the device record with the provided values
- **THEN** the system SHALL respond with the updated device information including `device_id`, `role`, and `preference`

#### Scenario: Device does not exist
- **WHEN** a preference update is sent for an unknown `device_id`
- **THEN** the system SHALL create the device record with the provided preference information

### Requirement: Pipeline tasks can be created and executed
The system SHALL provide an API to trigger a content processing pipeline for a device. The pipeline processes RSS items within a specified time range through five stages: Level 1 filter, crawl, summarize, Level 2 filter, and compose.

#### Scenario: Creating a task
- **WHEN** a client sends `POST /v1/device/{device_id}/task` with `time_range_start` and `time_range_end`
- **THEN** the system SHALL create a Task record with status `pending`
- **THEN** the system SHALL begin executing the pipeline asynchronously
- **THEN** the system SHALL respond with `201 Created` and the task details

#### Scenario: Time range validation
- **WHEN** a task request has invalid or missing time range parameters
- **THEN** the system SHALL reject the request with `400 Bad Request`

#### Scenario: Concurrent task rejection
- **WHEN** a task creation request is made while another task is already running
- **THEN** the system SHALL reject the request with `409 Conflict`

### Requirement: Pipeline progresses through defined stages
The system SHALL execute the pipeline stages in order, updating the task status at each stage: `filtering_l1` → `crawling` → `summarizing` → `filtering_l2` → `composing` → `completed`. On any stage failure, the task status SHALL be set to `failed` with an error message.

#### Scenario: Successful pipeline completion
- **WHEN** all five stages complete without errors
- **THEN** the task status SHALL be `completed`
- **THEN** the `completed_at` timestamp SHALL be set
- **THEN** the `report` field SHALL contain the generated Markdown report

#### Scenario: Stage failure
- **WHEN** any stage encounters an error
- **THEN** the task status SHALL be set to `failed`
- **THEN** the `error` field SHALL contain a description of the failure
- **THEN** no further stages SHALL be executed

### Requirement: Pipeline stages use Python scripts via file-based interface
Each pipeline stage SHALL be implemented by writing an input JSON file, executing a Python script (`pipeline.py`) with the appropriate `--mode` flag, and reading the output JSON file. All intermediate files SHALL be preserved for debugging.

#### Scenario: File-based communication
- **WHEN** a pipeline stage is executed
- **THEN** the Go backend SHALL write an input JSON file to `{data_dir}/{task_id}_{stage}_in.json`
- **THEN** the Go backend SHALL invoke `python3 pipeline.py --mode {stage} --input {input_path} --output {output_path}`
- **THEN** the Go backend SHALL read the output JSON file from `{data_dir}/{task_id}_{stage}_out.json`

### Requirement: Level 1 filter selects items by preference
Stage 1 (filter_l1) SHALL use LLM to filter items based on the device's role and preference. Items within the specified time range are evaluated by title and URL to determine relevance to the user's interests.

#### Scenario: Level 1 filter execution
- **WHEN** the filter_l1 stage executes
- **THEN** the input SHALL include device preference and all items (id, title, url) within the time range
- **THEN** the output SHALL be a list of item IDs deemed relevant to the user's preference
- **THEN** the result SHALL be stored in `task.level1_ids`

### Requirement: Crawler fetches full article content with rate limiting
Stage 2 (crawl) SHALL fetch full article content for each Level 1 item using crawl4ai, converting pages to Markdown. The crawler SHALL enforce per-domain rate limiting with random delays between 30 and 90 seconds.

#### Scenario: Successful crawl
- **WHEN** the crawl stage processes an item URL
- **THEN** the system SHALL fetch the page content and convert it to Markdown
- **THEN** the content SHALL be stored in `Item.content`

#### Scenario: Crawl failure for individual item
- **WHEN** a crawl fails for an individual item (timeout, network error, etc.)
- **THEN** that item SHALL be marked as failed in the output
- **THEN** the pipeline SHALL continue with remaining items
- **THEN** failed items SHALL NOT block subsequent stages

#### Scenario: Per-domain rate limiting
- **WHEN** the crawler accesses URLs from the same domain
- **THEN** there SHALL be a minimum delay between 30 and 90 seconds (randomly chosen) between consecutive requests to the same domain
- **THEN** rate limit state SHALL be persisted across tasks

### Requirement: Summarizer generates item abstracts
Stage 3 (summarize) SHALL use LLM to generate a concise abstract (~200 characters) for each Level 1 item that has been successfully crawled.

#### Scenario: Summarize execution
- **WHEN** the summarize stage executes
- **THEN** the input SHALL include items with their titles and crawled content
- **THEN** each item SHALL receive an abstract
- **THEN** abstracts SHALL be stored in `Item.abstract`

### Requirement: Level 2 filter refines selection
Stage 4 (filter_l2) SHALL use LLM to further filter items based on abstracts and the device preference, producing a refined Level 2 list.

#### Scenario: Level 2 filter execution
- **WHEN** the filter_l2 stage executes
- **THEN** the input SHALL include device preference and items with their titles and abstracts
- **THEN** the output SHALL be a refined list of item IDs
- **THEN** the result SHALL be stored in `task.level2_ids`

### Requirement: Compose generates a Markdown report
Stage 5 (compose) SHALL use LLM to generate a structured Markdown report from Level 2 items, tailored to the user's preference.

#### Scenario: Compose execution
- **WHEN** the compose stage executes
- **THEN** the input SHALL include device preference and Level 2 items with titles, abstracts, and URLs
- **THEN** the output SHALL be a Markdown report
- **THEN** the report SHALL be stored in `task.report`

### Requirement: Task status can be queried
The system SHALL provide endpoints to query task status and results.

#### Scenario: Query latest task
- **WHEN** a client sends `GET /v1/device/{device_id}/task`
- **THEN** the system SHALL return the most recent task for that device
- **THEN** the response SHALL include all task fields including `status`, `level1_ids`, `level2_ids`, `report` (when completed), and `error` (when failed)

#### Scenario: Query specific task
- **WHEN** a client sends `GET /v1/device/{device_id}/task/{task_id}`
- **THEN** the system SHALL return the specified task

#### Scenario: No tasks exist
- **WHEN** a device has no tasks
- **THEN** the system SHALL respond with `404 Not Found`