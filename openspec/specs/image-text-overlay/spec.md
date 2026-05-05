## Requirements

### Requirement: Rendered item images include title and UTC publication time overlays
Every processed 320x240 JPEG image SHALL render a semi-transparent black bar at the bottom containing the item title in white text and the publication time on the final line in UTC.

#### Scenario: Overlay is applied to a downloaded image
- **WHEN** an item image is successfully downloaded and rendered
- **THEN** the returned image includes the title and UTC publication time overlay

#### Scenario: Overlay is applied to a fallback image
- **WHEN** an item image is generated from a fallback color card
- **THEN** the returned image includes the title and UTC publication time overlay

#### Scenario: Publication time is present
- **WHEN** the item has a `PublishedAt` timestamp
- **THEN** the overlay renders the timestamp in UTC using the `2006-01-02 15:04` format

#### Scenario: Publication time is missing
- **WHEN** the item does not have a `PublishedAt` timestamp
- **THEN** the overlay renders the current processing time in UTC using the `2006-01-02 15:04` format

### Requirement: Overlay title text wraps to at most three lines
The title portion of the overlay SHALL wrap across character or word boundaries as needed, using up to three lines before truncating the final line with an ellipsis.

#### Scenario: Title fits on one line
- **WHEN** the title fits within the available overlay width
- **THEN** the title is rendered on a single line

#### Scenario: Title requires wrapping
- **WHEN** the title exceeds the available overlay width but fits within three wrapped lines
- **THEN** the title is rendered across multiple lines without exceeding three lines

#### Scenario: Title exceeds the maximum line count
- **WHEN** the title would exceed three wrapped lines
- **THEN** only the first three lines are rendered
- **AND** the final rendered line ends with an ellipsis that still fits within the available width

### Requirement: Overlay bar height adjusts to rendered content
The bottom overlay bar SHALL grow to fit the rendered title line count plus the time line while remaining bottom-aligned with the image.

#### Scenario: Overlay height matches content
- **WHEN** the title renders as `N` lines where `1 <= N <= 3`
- **THEN** the bar height is calculated from those `N` title lines plus one time line
- **AND** the bar remains aligned to the bottom edge of the image

#### Scenario: Overlay opacity remains readable
- **WHEN** overlay text is rendered
- **THEN** the bottom bar uses a semi-transparent black background wide enough to span the image width
