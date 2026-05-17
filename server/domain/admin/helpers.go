package admin

import (
	"cmp"
	"slices"
	"strings"
	"time"
)

type itemSortOption string

const (
	itemSortTimeDesc    itemSortOption = "time_desc"
	itemSortTimeAsc     itemSortOption = "time_asc"
	itemSortShowsDesc   itemSortOption = "shows_desc"
	itemSortShowsAsc    itemSortOption = "shows_asc"
	itemSortReadsDesc   itemSortOption = "reads_desc"
	itemSortReadsAsc    itemSortOption = "reads_asc"
	itemSortRatingsDesc itemSortOption = "ratings_desc"
	itemSortRatingsAsc  itemSortOption = "ratings_asc"
)

func paginateSlice[T any](items []T, page, pageSize int, pageURL func(int) string) ([]T, Pagination) {
	total := len(items)
	if pageSize <= 0 {
		pageSize = 20
	}
	totalPages := total / pageSize
	if total%pageSize != 0 {
		totalPages++
	}
	if totalPages == 0 {
		totalPages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}
	paged := items
	if total > 0 {
		paged = items[start:end]
	} else {
		paged = []T{}
	}

	pagination := Pagination{
		Page:       page,
		PageSize:   pageSize,
		TotalItems: total,
		TotalPages: totalPages,
		HasPrev:    page > 1,
		HasNext:    page < totalPages,
		Start:      0,
		End:        0,
	}
	if total > 0 {
		pagination.Start = start + 1
		pagination.End = end
	}
	if pagination.HasPrev {
		pagination.PrevURL = pageURL(page - 1)
	}
	if pagination.HasNext {
		pagination.NextURL = pageURL(page + 1)
	}

	return paged, pagination
}

func applyItemFilters(items []ItemSummary, filters ItemFilters) []ItemSummary {
	filtered := make([]ItemSummary, 0, len(items))
	var fromTime, toTime time.Time
	var hasFrom, hasTo bool
	if filters.From != "" {
		fromTime, _ = time.Parse("2006-01-02", filters.From)
		hasFrom = true
	}
	if filters.To != "" {
		toTime, _ = time.Parse("2006-01-02", filters.To)
		toTime = toTime.Add(24*time.Hour - time.Nanosecond)
		hasTo = true
	}
	titleNeedle := strings.ToLower(filters.Title)

	for _, item := range items {
		if filters.FeedID != 0 && item.FeedID != filters.FeedID {
			continue
		}
		if titleNeedle != "" && !strings.Contains(strings.ToLower(item.Title), titleNeedle) {
			continue
		}
		if hasFrom && item.DisplayTime.Before(fromTime) {
			continue
		}
		if hasTo && item.DisplayTime.After(toTime) {
			continue
		}
		filtered = append(filtered, item)
	}

	return filtered
}

func applyItemSort(items []ItemSummary, sort itemSortOption) {
	slices.SortStableFunc(items, func(a, b ItemSummary) int {
		switch sort {
		case itemSortTimeAsc:
			if c := a.DisplayTime.Compare(b.DisplayTime); c != 0 {
				return c
			}
		case itemSortShowsDesc:
			if c := cmp.Compare(b.ShowCount, a.ShowCount); c != 0 {
				return c
			}
		case itemSortShowsAsc:
			if c := cmp.Compare(a.ShowCount, b.ShowCount); c != 0 {
				return c
			}
		case itemSortReadsDesc:
			if c := cmp.Compare(b.ReadCount, a.ReadCount); c != 0 {
				return c
			}
		case itemSortReadsAsc:
			if c := cmp.Compare(a.ReadCount, b.ReadCount); c != 0 {
				return c
			}
		case itemSortRatingsDesc:
			if c := cmp.Compare(b.RatingCount, a.RatingCount); c != 0 {
				return c
			}
		case itemSortRatingsAsc:
			if c := cmp.Compare(a.RatingCount, b.RatingCount); c != 0 {
				return c
			}
		default:
			if c := b.DisplayTime.Compare(a.DisplayTime); c != 0 {
				return c
			}
		}

		if c := cmp.Compare(b.ID, a.ID); c != 0 {
			return c
		}
		return 0
	})
}

func NormalizeItemSort(raw string) string {
	sort := itemSortOption(raw)
	switch sort {
	case itemSortTimeDesc, itemSortTimeAsc, itemSortShowsDesc, itemSortShowsAsc, itemSortReadsDesc, itemSortReadsAsc, itemSortRatingsDesc, itemSortRatingsAsc:
		return string(sort)
	default:
		return string(itemSortTimeDesc)
	}
}