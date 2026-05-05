## MODIFIED Requirements

### Requirement: Rating records preserve submission details
The system SHALL store each accepted rating as a database record that identifies the rated item, the submitted device identifier when provided, the submitted rating value, and when the rating was recorded. Rating records SHALL remain distinct from item read records and SHALL NOT imply that an NFC read occurred.

#### Scenario: Rating is persisted with required fields
- **WHEN** the system accepts a rating request
- **THEN** the stored record SHALL include the target item ID
- **THEN** the stored record SHALL include the submitted device identifier when one is provided
- **THEN** the stored record SHALL include the submitted rating value
- **THEN** the stored record SHALL include a creation timestamp

#### Scenario: Rating does not create a read record
- **WHEN** the system accepts a rating request for an item
- **THEN** the system persists the rating record
- **THEN** the system does not treat that rating record as an item read record
