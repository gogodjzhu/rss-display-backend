## Requirements

### Requirement: NFC redirects create read records
The system SHALL persist an item read record only when an NFC redirect request successfully resolves a device's current item and issues the redirect response.

#### Scenario: Read is recorded on successful redirect
- **WHEN** a user requests `GET /nfc/{device_id}` for a device with a valid current item and item URL
- **THEN** the system persists a read record containing the device identifier and item identifier
- **THEN** the system responds with the redirect to the item's source URL

#### Scenario: Read is not recorded when device does not exist
- **WHEN** a user requests `GET /nfc/{device_id}` for an unknown device
- **THEN** the system does not persist a read record
- **THEN** the system responds with `404 Not Found`

#### Scenario: Read is not recorded when current item is missing
- **WHEN** a user requests `GET /nfc/{device_id}` for a device without a current item or with a current item that cannot be loaded
- **THEN** the system does not persist a read record
- **THEN** the system responds with `404 Not Found`

### Requirement: Read records preserve read semantics
The system SHALL treat a read record as evidence of a successful NFC open and SHALL NOT infer reads from device polling or item ratings.

#### Scenario: Polling does not imply a read
- **WHEN** a device requests `GET /v1/device/{device_id}/next`
- **THEN** the system updates device state as needed
- **THEN** the system does not persist a read record for the selected item

#### Scenario: Rating does not imply a read
- **WHEN** a device submits a valid rating for an item
- **THEN** the system persists the rating record
- **THEN** the system does not create a read record unless an NFC redirect has also succeeded
