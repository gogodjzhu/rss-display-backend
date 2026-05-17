package api

import "github.com/esp32-rss-display/backend/server/domain/items"

// Re-exported constants kept for backward compatibility with existing tests and handlers.
const PlaceholderItemID = items.PlaceholderItemID
const PlaceholderTitle = items.PlaceholderTitle

// NextItemSelector is a backward-compatibility alias for items.ItemSelector.
// New code should use items.ItemSelector directly.
type NextItemSelector = items.ItemSelector

// WeightedNextItemSelector is a backward-compatibility alias for items.WeightedItemSelector.
type WeightedNextItemSelector = items.WeightedItemSelector

// NewWeightedNextItemSelector is a backward-compatibility constructor.
func NewWeightedNextItemSelector() *WeightedNextItemSelector {
	return items.NewWeightedItemSelector()
}
