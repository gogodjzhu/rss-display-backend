## Requirements

### Requirement: Fallback images are generated when no usable item image exists
The system SHALL generate a 320x240 fallback color-card image for an item whenever no usable upstream image is available, and SHALL continue rendering the final image instead of returning an empty image result.

#### Scenario: Item has no image
- **WHEN** an item has no extracted upstream image URL
- **THEN** the system generates a fallback color-card image for that item

#### Scenario: Image processing fails
- **WHEN** an item has an upstream image URL but download, decode, or image processing fails
- **THEN** the system generates a fallback color-card image for that item instead of returning no image

### Requirement: Fallback color cards are deterministic from the item title
The fallback color-card background SHALL be selected deterministically from a predefined palette using a hash of the item title so that the same title always produces the same fallback color.

#### Scenario: Same title yields the same fallback color
- **WHEN** two items have identical titles
- **THEN** their generated fallback color cards use the same background color

#### Scenario: Palette selection uses the title hash
- **WHEN** a fallback color card is generated
- **THEN** the selected background color is chosen from the predefined palette using the item title hash

### Requirement: Fallback color cards use a dark readable palette
The predefined fallback palette SHALL consist of dark colors suitable for white overlay text readability.

#### Scenario: Generated fallback remains readable under white text
- **WHEN** any fallback color card is generated
- **THEN** its background color comes from the dark fallback palette
