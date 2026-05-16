# Add RSS Content Pipeline

## Problem

The system currently collects RSS items with only titles, URLs, and images. There is no mechanism to filter, enrich, or summarize content based on user interests. ESP32 devices receive random items from all feeds without any personalization or content analysis.

## Proposal

Add a 5-stage content pipeline that:

1. **Filter (Level 1)** — Given a user's preference profile, use LLM to select relevant items from a time range based on titles alone.
2. **Crawl** — For each Level 1 item, fetch the full article content via crawl4ai (Python), converting to Markdown, with per-domain rate limiting (30–90s random delay).
3. **Summarize** — For each crawled item, use LLM to generate an abstract (~200 chars).
4. **Filter (Level 2)** — Re-evaluate Level 1 items using their abstracts plus user preference, producing a refined Level 2 list.
5. **Compose** — Generate a Markdown report from Level 2 items tailored to the user's preference.

The pipeline is triggered via API, runs asynchronously, and stores all intermediate results for debugging.

## User-Facing Changes

- New `PUT /v1/device/{device_id}/preference` endpoint — set device role and preference text.
- New `POST /v1/device/{device_id}/task` endpoint — trigger a pipeline run with explicit time range.
- New `GET /v1/device/{device_id}/task` and `GET /v1/device/{device_id}/task/{task_id}` — query task status and report.
- `Item` model gains `content` and `abstract` fields (populated by pipeline).
- `Device` model gains `role` and `preference` fields.

## Non-Goals

- Automatic/scheduled pipeline triggering (manual API only for now).
- Multiple concurrent tasks (only one task can run at a time globally).
- ESP32 display of reports (report is stored in DB for future use).
- Training or fine-tuning custom LLM models.