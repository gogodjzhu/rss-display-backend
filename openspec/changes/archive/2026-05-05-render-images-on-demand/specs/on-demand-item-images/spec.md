## ADDED Requirements

### Requirement: Every item has a local image endpoint
The system SHALL expose a backend-managed image URL for every item returned by `GET /v1/device/{device_id}/next`, using the `/image/{item_id}.jpg` route regardless of whether an upstream image URL was extracted from the RSS item.

#### Scenario: Item with upstream image URL
- **WHEN** a device requests the next item for an RSS entry whose stored upstream image URL is non-empty
- **THEN** the response includes a non-empty backend `image_url` pointing to `/image/{item_id}.jpg`

#### Scenario: Item without upstream image URL
- **WHEN** a device requests the next item for an RSS entry whose stored upstream image URL is empty
- **THEN** the response still includes a non-empty backend `image_url` pointing to `/image/{item_id}.jpg`

### Requirement: Image handler renders upstream images on demand
The system SHALL fetch and render the upstream image only when `/image/{item_id}.jpg` is requested, and SHALL NOT pre-download or persist processed item images during RSS polling.

#### Scenario: Polling stores only source image data
- **WHEN** the RSS worker stores a newly discovered item
- **THEN** it saves the extracted upstream image URL as item metadata
- **AND** it does not download, resize, overlay, or write a processed image file for that item

#### Scenario: Request-time rendering of upstream image
- **WHEN** `/image/{item_id}.jpg` is requested for an item with a valid upstream image URL
- **THEN** the handler downloads the upstream image at request time
- **AND** rescales it to the configured output dimensions before returning a JPEG response

### Requirement: Image rendering falls back to a color card
The system SHALL return a generated color-card image with title and item publication time overlay whenever an item has no upstream image URL or request-time image retrieval/rendering fails.

#### Scenario: Item has no upstream image URL
- **WHEN** `/image/{item_id}.jpg` is requested for an item whose stored upstream image URL is empty
- **THEN** the handler returns a generated color-card JPEG instead of an error response

#### Scenario: Upstream image download times out
- **WHEN** `/image/{item_id}.jpg` is requested and the upstream image download exceeds the configured timeout
- **THEN** the handler returns a generated color-card JPEG instead of propagating the timeout as a failure response

#### Scenario: Upstream image cannot be decoded
- **WHEN** `/image/{item_id}.jpg` is requested and the upstream image download succeeds but image decoding or supported-format validation fails
- **THEN** the handler returns a generated color-card JPEG instead of an error response

### Requirement: Request-time image download timeout is configurable
The system SHALL load the upstream image download timeout from configuration and apply it to request-time image downloads.

#### Scenario: Configured timeout is applied
- **WHEN** the service is configured with `rss.image_download_timeout_seconds`
- **THEN** request-time upstream image downloads use that timeout value

#### Scenario: Three-second timeout configuration
- **WHEN** the configuration sets `rss.image_download_timeout_seconds` to `3`
- **THEN** the image handler limits each upstream image download attempt to 3 seconds before falling back to a color card

### Requirement: Image rendering is observable
The system SHALL emit logs and metrics for request-time image rendering attempts and color-card fallbacks.

#### Scenario: Successful render is tracked
- **WHEN** `/image/{item_id}.jpg` returns a rendered image response
- **THEN** the system records a metric for the image render request

#### Scenario: Fallback is tracked
- **WHEN** `/image/{item_id}.jpg` falls back to a color card because the upstream image is missing or rendering fails
- **THEN** the system records fallback-related metrics and logs the failure reason when an upstream image was attempted
