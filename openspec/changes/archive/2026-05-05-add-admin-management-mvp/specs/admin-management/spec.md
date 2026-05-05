## ADDED Requirements

### Requirement: The system provides an internal admin dashboard
The system SHALL provide a server-rendered admin dashboard at `/admin` that summarizes feeds, items, devices, shows, reads, and ratings using persisted backend data.

#### Scenario: Dashboard loads successfully
- **WHEN** a user requests `/admin`
- **THEN** the system returns an HTML page
- **THEN** the page includes summary values for total feeds, total items, total devices, total shows, total reads, and total ratings

### Requirement: The system provides feed management pages
The system SHALL provide admin pages for listing feeds and inspecting a single feed with item-level details and feed-level aggregates.

#### Scenario: Feed list is shown
- **WHEN** a user requests `/admin/feeds`
- **THEN** the system returns an HTML page listing persisted feeds
- **THEN** each feed row includes its enabled state and aggregate item, show, read, and rating metrics

#### Scenario: Feed list is paginated
- **WHEN** a user requests `/admin/feeds` and the persisted feed count exceeds one page
- **THEN** the system returns only the feeds for the requested page
- **THEN** the page includes navigation for adjacent pages

#### Scenario: Feed detail is shown
- **WHEN** a user requests `/admin/feeds/{id}` for an existing feed
- **THEN** the system returns an HTML page showing that feed's metadata
- **THEN** the page includes a list of that feed's items with per-item show, read, and rating metrics

#### Scenario: Feed detail items are paginated
- **WHEN** a user requests `/admin/feeds/{id}` for a feed with more items than fit on one page
- **THEN** the system returns only the item rows for the requested page
- **THEN** the page includes navigation for adjacent pages

### Requirement: The system provides device management pages
The system SHALL provide admin pages for listing devices and inspecting a single device with separate show, read, and rating records.

#### Scenario: Device list is shown
- **WHEN** a user requests `/admin/devices`
- **THEN** the system returns an HTML page listing persisted devices
- **THEN** each device row includes its device identifier, last seen time, show count, read count, and rating count

#### Scenario: Device list is paginated
- **WHEN** a user requests `/admin/devices` and the persisted device count exceeds one page
- **THEN** the system returns only the devices for the requested page
- **THEN** the page includes navigation for adjacent pages

#### Scenario: Device detail is shown
- **WHEN** a user requests `/admin/devices/{device_id}` for an existing device
- **THEN** the system returns an HTML page showing the device's identifier, current item, and last seen time
- **THEN** the page includes show records, read records, and rating records in separate sections

### Requirement: The system provides item management pages
The system SHALL provide admin pages for listing items and inspecting a single item with related show, read, and rating details.

#### Scenario: Item list is shown
- **WHEN** a user requests `/admin/items`
- **THEN** the system returns an HTML page listing persisted items
- **THEN** each item row includes its feed, publication or creation time, show count, read count, and rating metrics

#### Scenario: Item list is paginated
- **WHEN** a user requests `/admin/items` and the persisted item count exceeds one page
- **THEN** the system returns only the items for the requested page
- **THEN** the page includes navigation for adjacent pages

#### Scenario: Item list supports filters and sorting
- **WHEN** a user requests `/admin/items` with supported feed, title, or time-range filters and a supported sort field
- **THEN** the system applies those filters before rendering the results
- **THEN** the system orders the results by the requested field before pagination is applied

#### Scenario: Item detail is shown
- **WHEN** a user requests `/admin/items/{id}` for an existing item
- **THEN** the system returns an HTML page showing the item's metadata and source link
- **THEN** the page includes the devices that were shown the item, the devices that read the item, and the ratings submitted for the item
