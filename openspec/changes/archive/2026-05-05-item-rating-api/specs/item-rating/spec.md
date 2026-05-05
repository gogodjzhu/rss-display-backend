## ADDED Requirements

### Requirement: Next item responses include the item identifier
The system SHALL include an `item_id` field in every `/v1/device/{device_id}/next` JSON response so clients can reference the displayed item in later API calls.

#### Scenario: Normal item is returned
- **WHEN** a device requests `/v1/device/{device_id}/next` and a persisted item is selected
- **THEN** the response SHALL include that item's numeric `item_id`

#### Scenario: Placeholder item is returned during initial sync
- **WHEN** a device requests `/v1/device/{device_id}/next` before any persisted items exist
- **THEN** the response SHALL still include an `item_id` field
- **THEN** the placeholder `item_id` SHALL be `0`

### Requirement: Clients can submit a rating for an item
The system SHALL provide an API for submitting a rating from 1 to 5 for a specific item and SHALL persist accepted ratings in the database.

#### Scenario: Rating submission succeeds
- **WHEN** a client sends a valid rating request for an existing item with a rating value from 1 to 5
- **THEN** the system SHALL persist a rating record for that item
- **THEN** the system SHALL respond with a success status

#### Scenario: Item does not exist
- **WHEN** a client sends a rating request for an unknown item ID
- **THEN** the system SHALL reject the request with `404 Not Found`

#### Scenario: Rating value is out of range
- **WHEN** a client sends a rating request with a value below 1 or above 5
- **THEN** the system SHALL reject the request with `400 Bad Request`

#### Scenario: Placeholder item cannot be rated
- **WHEN** a client sends a rating request for placeholder item ID `0`
- **THEN** the system SHALL reject the request with `400 Bad Request`

### Requirement: Rating records preserve submission details
The system SHALL store each accepted rating as a database record that identifies the rated item, the submitted rating value, and when the rating was recorded.

#### Scenario: Rating is persisted with required fields
- **WHEN** the system accepts a rating request
- **THEN** the stored record SHALL include the target item ID
- **THEN** the stored record SHALL include the submitted rating value
- **THEN** the stored record SHALL include a creation timestamp
