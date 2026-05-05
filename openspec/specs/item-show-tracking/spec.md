## Requirements

### Requirement: Next item responses create show records
The system SHALL persist an item show record only when `GET /v1/device/{device_id}/next` successfully selects a persisted item and updates device state for that item.

#### Scenario: Show is recorded for a persisted item
- **WHEN** a device successfully requests `GET /v1/device/{device_id}/next` and the selected item is a persisted non-placeholder item
- **THEN** the system persists a show record containing the device identifier and item identifier
- **THEN** the response still includes that item in the JSON payload

#### Scenario: Placeholder item does not create a show record
- **WHEN** a device requests `GET /v1/device/{device_id}/next` and the system returns the placeholder item
- **THEN** the system does not persist a show record

#### Scenario: Failed next-item request does not create a show record
- **WHEN** `GET /v1/device/{device_id}/next` fails before the response is produced
- **THEN** the system does not persist a show record

### Requirement: Show records preserve show semantics
The system SHALL treat a show record as evidence that the backend served an item through the next-item API and SHALL NOT infer reads or ratings from show records alone.

#### Scenario: Show does not imply read
- **WHEN** the system persists a show record for an item
- **THEN** that record does not count as an item read unless an NFC redirect also succeeds

#### Scenario: Show does not imply rating
- **WHEN** the system persists a show record for an item
- **THEN** that record does not count as an item rating unless a rating submission is also accepted
