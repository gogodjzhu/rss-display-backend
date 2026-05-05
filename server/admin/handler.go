package admin

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/esp32-rss-display/backend/server/database"
	"github.com/esp32-rss-display/backend/server/models"
	"gorm.io/gorm"
)

const pageSize = 20

type Handler struct {
	templates *template.Template
}

type layoutData struct {
	Title      string
	ActivePath string
	Content    template.HTML
}

type DashboardSummary struct {
	TotalFeeds   int64
	TotalItems   int64
	TotalDevices int64
	TotalShows   int64
	TotalReads   int64
	TotalRatings int64
}

type FeedSummary struct {
	ID            uint
	Name          string
	URL           string
	Enabled       bool
	CreatedAt     time.Time
	ItemCount     int64
	ShowCount     int64
	ReadCount     int64
	RatingCount   int64
	AverageRating float64
	LastItemAt    *time.Time
}

type ItemSummary struct {
	ID            uint
	FeedID        uint
	FeedName      string
	Title         string
	URL           string
	PublishedAt   *time.Time
	CreatedAt     time.Time
	DisplayTime   time.Time
	ShowCount     int64
	ReadCount     int64
	RatingCount   int64
	AverageRating float64
}

type DeviceSummary struct {
	DeviceID         string
	CurrentItemValue uint
	HasCurrentItem   bool
	CurrentItemID    *uint
	CurrentItemTitle string
	LastSeen         time.Time
	CreatedAt        time.Time
	ShowCount        int64
	ReadCount        int64
	RatingCount      int64
	LastShowAt       *time.Time
	LastReadAt       *time.Time
	LastRatingAt     *time.Time
}

type ShowRecordView struct {
	CreatedAt time.Time
	DeviceID  string
	ItemID    uint
	ItemTitle string
	ItemURL   string
	FeedName  string
}

type ReadRecordView struct {
	CreatedAt time.Time
	DeviceID  string
	ItemID    uint
	ItemTitle string
	ItemURL   string
	FeedName  string
}

type RatingRecordView struct {
	CreatedAt time.Time
	DeviceID  string
	ItemID    uint
	ItemTitle string
	FeedName  string
	Rating    int
}

type Pagination struct {
	Page       int
	PageSize   int
	TotalItems int
	TotalPages int
	HasPrev    bool
	HasNext    bool
	PrevURL    string
	NextURL    string
	Start      int
	End        int
}

type DashboardView struct {
	Summary       DashboardSummary
	RecentItems   []ItemSummary
	RecentShows   []ShowRecordView
	RecentReads   []ReadRecordView
	RecentRatings []RatingRecordView
	RecentDevices []DeviceSummary
}

type FeedListView struct {
	Feeds      []FeedSummary
	Pagination Pagination
}

type FeedDetailView struct {
	Feed       FeedSummary
	Items      []ItemSummary
	Pagination Pagination
}

type ItemFilters struct {
	FeedID    uint
	Title     string
	From      string
	To        string
	Sort      string
	FeedNames []FeedOption
}

type FeedOption struct {
	ID   uint
	Name string
}

type ItemListView struct {
	Items      []ItemSummary
	Filters    ItemFilters
	Pagination Pagination
}

type ItemDetailView struct {
	Item    ItemSummary
	Shows   []ShowRecordView
	Reads   []ReadRecordView
	Ratings []RatingRecordView
}

type DeviceListView struct {
	Devices    []DeviceSummary
	Pagination Pagination
}

type DeviceDetailView struct {
	Device  DeviceSummary
	Shows   []ShowRecordView
	Reads   []ReadRecordView
	Ratings []RatingRecordView
}

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

func NewHandler() *Handler {
	return &Handler{templates: mustParseTemplates()}
}

func Mount(mux *http.ServeMux) {
	h := NewHandler()
	mux.HandleFunc("GET /admin", h.Dashboard)
	mux.HandleFunc("GET /admin/feeds", h.ListFeeds)
	mux.HandleFunc("GET /admin/feeds/{id}", h.ShowFeed)
	mux.HandleFunc("GET /admin/items", h.ListItems)
	mux.HandleFunc("GET /admin/items/{id}", h.ShowItem)
	mux.HandleFunc("GET /admin/devices", h.ListDevices)
	mux.HandleFunc("GET /admin/devices/{device_id}", h.ShowDevice)
}

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	db := database.GetDB()
	view, err := h.buildDashboardView(db)
	if err != nil {
		http.Error(w, "failed to load dashboard", http.StatusInternalServerError)
		return
	}

	h.render(w, "Dashboard", "/admin", "dashboard", view)
}

func (h *Handler) ListFeeds(w http.ResponseWriter, r *http.Request) {
	db := database.GetDB()
	feeds, err := h.listFeedSummaries(db)
	if err != nil {
		http.Error(w, "failed to load feeds", http.StatusInternalServerError)
		return
	}

	page := parsePage(r)
	paged, pagination := paginateSlice(feeds, page, pageSize, func(page int) string {
		return buildPageURL(r, page)
	})

	h.render(w, "Feeds", "/admin/feeds", "feeds_list", FeedListView{Feeds: paged, Pagination: pagination})
}

func (h *Handler) ShowFeed(w http.ResponseWriter, r *http.Request) {
	feedID, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	db := database.GetDB()
	var feed models.Feed
	if err := db.First(&feed, uint(feedID)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "failed to load feed", http.StatusInternalServerError)
		return
	}

	items, err := h.listItemSummaries(db)
	if err != nil {
		http.Error(w, "failed to load feed items", http.StatusInternalServerError)
		return
	}
	filtered := make([]ItemSummary, 0)
	for _, item := range items {
		if item.FeedID == feed.ID {
			filtered = append(filtered, item)
		}
	}

	page := parsePage(r)
	paged, pagination := paginateSlice(filtered, page, pageSize, func(page int) string {
		return buildPageURL(r, page)
	})

	view := FeedDetailView{
		Feed:       summarizeFeed(feed, filtered),
		Items:      paged,
		Pagination: pagination,
	}
	h.render(w, fmt.Sprintf("Feed %s", feed.Name), "/admin/feeds", "feed_detail", view)
}

func (h *Handler) ListItems(w http.ResponseWriter, r *http.Request) {
	db := database.GetDB()
	items, err := h.listItemSummaries(db)
	if err != nil {
		http.Error(w, "failed to load items", http.StatusInternalServerError)
		return
	}

	filters, err := h.parseItemFilters(db, r)
	if err != nil {
		http.Error(w, "invalid item filters", http.StatusBadRequest)
		return
	}
	items = applyItemFilters(items, filters)
	applyItemSort(items, itemSortOption(filters.Sort))

	page := parsePage(r)
	paged, pagination := paginateSlice(items, page, pageSize, func(page int) string {
		return buildPageURL(r, page)
	})

	h.render(w, "Items", "/admin/items", "items_list", ItemListView{Items: paged, Filters: filters, Pagination: pagination})
}

func (h *Handler) ShowItem(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	db := database.GetDB()
	items, err := h.listItemSummaries(db)
	if err != nil {
		http.Error(w, "failed to load item", http.StatusInternalServerError)
		return
	}
	var selected *ItemSummary
	for i := range items {
		if items[i].ID == uint(itemID) {
			selected = &items[i]
			break
		}
	}
	if selected == nil {
		http.NotFound(w, r)
		return
	}

	shows, err := h.queryShowRecords(db, func(query *gorm.DB) *gorm.DB {
		return query.Where("item_shows.item_id = ?", uint(itemID))
	}, 0)
	if err != nil {
		http.Error(w, "failed to load item shows", http.StatusInternalServerError)
		return
	}
	reads, err := h.queryReadRecords(db, func(query *gorm.DB) *gorm.DB {
		return query.Where("item_reads.item_id = ?", uint(itemID))
	}, 0)
	if err != nil {
		http.Error(w, "failed to load item reads", http.StatusInternalServerError)
		return
	}
	ratings, err := h.queryRatingRecords(db, func(query *gorm.DB) *gorm.DB {
		return query.Where("item_ratings.item_id = ?", uint(itemID))
	}, 0)
	if err != nil {
		http.Error(w, "failed to load item ratings", http.StatusInternalServerError)
		return
	}

	h.render(w, fmt.Sprintf("Item %d", itemID), "/admin/items", "item_detail", ItemDetailView{
		Item:    *selected,
		Shows:   shows,
		Reads:   reads,
		Ratings: ratings,
	})
}

func (h *Handler) ListDevices(w http.ResponseWriter, r *http.Request) {
	db := database.GetDB()
	devices, err := h.listDeviceSummaries(db)
	if err != nil {
		http.Error(w, "failed to load devices", http.StatusInternalServerError)
		return
	}

	page := parsePage(r)
	paged, pagination := paginateSlice(devices, page, pageSize, func(page int) string {
		return buildPageURL(r, page)
	})

	h.render(w, "Devices", "/admin/devices", "devices_list", DeviceListView{Devices: paged, Pagination: pagination})
}

func (h *Handler) ShowDevice(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		http.NotFound(w, r)
		return
	}

	db := database.GetDB()
	devices, err := h.listDeviceSummaries(db)
	if err != nil {
		http.Error(w, "failed to load devices", http.StatusInternalServerError)
		return
	}

	var selected *DeviceSummary
	for i := range devices {
		if devices[i].DeviceID == deviceID {
			selected = &devices[i]
			break
		}
	}
	if selected == nil {
		http.NotFound(w, r)
		return
	}

	shows, err := h.queryShowRecords(db, func(query *gorm.DB) *gorm.DB {
		return query.Where("item_shows.device_id = ?", deviceID)
	}, 0)
	if err != nil {
		http.Error(w, "failed to load device shows", http.StatusInternalServerError)
		return
	}
	reads, err := h.queryReadRecords(db, func(query *gorm.DB) *gorm.DB {
		return query.Where("item_reads.device_id = ?", deviceID)
	}, 0)
	if err != nil {
		http.Error(w, "failed to load device reads", http.StatusInternalServerError)
		return
	}
	ratings, err := h.queryRatingRecords(db, func(query *gorm.DB) *gorm.DB {
		return query.Where("item_ratings.device_id = ?", deviceID)
	}, 0)
	if err != nil {
		http.Error(w, "failed to load device ratings", http.StatusInternalServerError)
		return
	}

	h.render(w, fmt.Sprintf("Device %s", deviceID), "/admin/devices", "device_detail", DeviceDetailView{
		Device:  *selected,
		Shows:   shows,
		Reads:   reads,
		Ratings: ratings,
	})
}

func (h *Handler) render(w http.ResponseWriter, title, activePath, bodyTemplate string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var body bytes.Buffer
	if err := h.templates.ExecuteTemplate(&body, bodyTemplate, data); err != nil {
		http.Error(w, "failed to render admin page", http.StatusInternalServerError)
		return
	}

	if err := h.templates.ExecuteTemplate(w, "layout", layoutData{
		Title:      title,
		ActivePath: activePath,
		Content:    template.HTML(body.String()),
	}); err != nil {
		http.Error(w, "failed to render admin page", http.StatusInternalServerError)
	}
}

func (h *Handler) buildDashboardView(db *gorm.DB) (DashboardView, error) {
	summary := DashboardSummary{}
	for _, query := range []struct {
		model  any
		target *int64
	}{
		{model: &models.Feed{}, target: &summary.TotalFeeds},
		{model: &models.Item{}, target: &summary.TotalItems},
		{model: &models.Device{}, target: &summary.TotalDevices},
		{model: &models.ItemShow{}, target: &summary.TotalShows},
		{model: &models.ItemRead{}, target: &summary.TotalReads},
		{model: &models.ItemRating{}, target: &summary.TotalRatings},
	} {
		if err := db.Model(query.model).Count(query.target).Error; err != nil {
			return DashboardView{}, err
		}
	}

	recentItems, err := h.listItemSummaries(db)
	if err != nil {
		return DashboardView{}, err
	}
	if len(recentItems) > 5 {
		recentItems = recentItems[:5]
	}
	recentShows, err := h.queryShowRecords(db, nil, 5)
	if err != nil {
		return DashboardView{}, err
	}
	recentReads, err := h.queryReadRecords(db, nil, 5)
	if err != nil {
		return DashboardView{}, err
	}
	recentRatings, err := h.queryRatingRecords(db, nil, 5)
	if err != nil {
		return DashboardView{}, err
	}
	recentDevices, err := h.listDeviceSummaries(db)
	if err != nil {
		return DashboardView{}, err
	}
	if len(recentDevices) > 5 {
		recentDevices = recentDevices[:5]
	}

	return DashboardView{
		Summary:       summary,
		RecentItems:   recentItems,
		RecentShows:   recentShows,
		RecentReads:   recentReads,
		RecentRatings: recentRatings,
		RecentDevices: recentDevices,
	}, nil
}

func (h *Handler) listFeedSummaries(db *gorm.DB) ([]FeedSummary, error) {
	var feeds []models.Feed
	if err := db.Order("name ASC, id ASC").Find(&feeds).Error; err != nil {
		return nil, err
	}

	items, err := h.listItemSummaries(db)
	if err != nil {
		return nil, err
	}
	itemsByFeed := make(map[uint][]ItemSummary, len(feeds))
	for _, item := range items {
		itemsByFeed[item.FeedID] = append(itemsByFeed[item.FeedID], item)
	}

	summaries := make([]FeedSummary, 0, len(feeds))
	for _, feed := range feeds {
		summaries = append(summaries, summarizeFeed(feed, itemsByFeed[feed.ID]))
	}

	return summaries, nil
}

func summarizeFeed(feed models.Feed, items []ItemSummary) FeedSummary {
	summary := FeedSummary{
		ID:        feed.ID,
		Name:      feed.Name,
		URL:       feed.URL,
		Enabled:   feed.Enabled,
		CreatedAt: feed.CreatedAt,
	}

	var ratingTotal float64
	for _, item := range items {
		summary.ItemCount++
		summary.ShowCount += item.ShowCount
		summary.ReadCount += item.ReadCount
		summary.RatingCount += item.RatingCount
		ratingTotal += item.AverageRating * float64(item.RatingCount)

		itemTime := item.DisplayTime
		if summary.LastItemAt == nil || itemTime.After(*summary.LastItemAt) {
			copyTime := itemTime
			summary.LastItemAt = &copyTime
		}
	}

	if summary.RatingCount > 0 {
		summary.AverageRating = ratingTotal / float64(summary.RatingCount)
	}

	return summary
}

func (h *Handler) listItemSummaries(db *gorm.DB) ([]ItemSummary, error) {
	var items []models.Item
	if err := db.Model(&models.Item{}).Order("COALESCE(published_at, created_at) DESC, id DESC").Find(&items).Error; err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return []ItemSummary{}, nil
	}

	itemIDs := make([]uint, 0, len(items))
	feedIDs := make([]uint, 0, len(items))
	seenFeedIDs := map[uint]struct{}{}
	for _, item := range items {
		itemIDs = append(itemIDs, item.ID)
		if _, ok := seenFeedIDs[item.FeedID]; !ok {
			feedIDs = append(feedIDs, item.FeedID)
			seenFeedIDs[item.FeedID] = struct{}{}
		}
	}

	feedsByID, err := loadFeedsByID(db, feedIDs)
	if err != nil {
		return nil, err
	}
	showCounts, err := loadItemShowCounts(db, itemIDs)
	if err != nil {
		return nil, err
	}
	readCounts, err := loadItemReadCounts(db, itemIDs)
	if err != nil {
		return nil, err
	}
	ratingAggs, err := loadItemRatingAggregates(db, itemIDs)
	if err != nil {
		return nil, err
	}

	summaries := make([]ItemSummary, 0, len(items))
	for _, item := range items {
		itemTime := item.CreatedAt
		if item.PublishedAt != nil {
			itemTime = *item.PublishedAt
		}
		agg := ratingAggs[item.ID]
		summaries = append(summaries, ItemSummary{
			ID:            item.ID,
			FeedID:        item.FeedID,
			FeedName:      feedsByID[item.FeedID].Name,
			Title:         item.Title,
			URL:           item.URL,
			PublishedAt:   item.PublishedAt,
			CreatedAt:     item.CreatedAt,
			DisplayTime:   itemTime,
			ShowCount:     showCounts[item.ID],
			ReadCount:     readCounts[item.ID],
			RatingCount:   agg.Count,
			AverageRating: agg.Average,
		})
	}

	return summaries, nil
}

func (h *Handler) listDeviceSummaries(db *gorm.DB) ([]DeviceSummary, error) {
	var devices []models.Device
	if err := db.Order("last_seen DESC, device_id ASC").Find(&devices).Error; err != nil {
		return nil, err
	}
	if len(devices) == 0 {
		return []DeviceSummary{}, nil
	}

	currentItemIDs := make([]uint, 0, len(devices))
	seenItemIDs := map[uint]struct{}{}
	for _, device := range devices {
		if device.CurrentItemID == nil {
			continue
		}
		if _, ok := seenItemIDs[*device.CurrentItemID]; ok {
			continue
		}
		seenItemIDs[*device.CurrentItemID] = struct{}{}
		currentItemIDs = append(currentItemIDs, *device.CurrentItemID)
	}

	itemsByID, err := loadItemsByID(db, currentItemIDs)
	if err != nil {
		return nil, err
	}
	showAggs, err := loadDeviceShowAggregates(db)
	if err != nil {
		return nil, err
	}
	readAggs, err := loadDeviceReadAggregates(db)
	if err != nil {
		return nil, err
	}
	ratingAggs, err := loadDeviceRatingAggregates(db)
	if err != nil {
		return nil, err
	}

	summaries := make([]DeviceSummary, 0, len(devices))
	for _, device := range devices {
		showAgg := showAggs[device.DeviceID]
		readAgg := readAggs[device.DeviceID]
		ratingAgg := ratingAggs[device.DeviceID]
		summary := DeviceSummary{
			DeviceID:      device.DeviceID,
			CurrentItemID: device.CurrentItemID,
			LastSeen:      device.LastSeen,
			CreatedAt:     device.CreatedAt,
			ShowCount:     showAgg.Count,
			ReadCount:     readAgg.Count,
			RatingCount:   ratingAgg.Count,
		}
		if !showAgg.LastAt.IsZero() {
			last := showAgg.LastAt
			summary.LastShowAt = &last
		}
		if !readAgg.LastAt.IsZero() {
			last := readAgg.LastAt
			summary.LastReadAt = &last
		}
		if !ratingAgg.LastAt.IsZero() {
			last := ratingAgg.LastAt
			summary.LastRatingAt = &last
		}
		if device.CurrentItemID != nil {
			summary.CurrentItemValue = *device.CurrentItemID
			summary.HasCurrentItem = true
			summary.CurrentItemTitle = itemsByID[*device.CurrentItemID].Title
		}
		summaries = append(summaries, summary)
	}

	return summaries, nil
}

func (h *Handler) parseItemFilters(db *gorm.DB, r *http.Request) (ItemFilters, error) {
	filters := ItemFilters{Sort: normalizeItemSort(r.URL.Query().Get("sort"))}
	if title := strings.TrimSpace(r.URL.Query().Get("title")); title != "" {
		filters.Title = title
	}
	if rawFeedID := strings.TrimSpace(r.URL.Query().Get("feed_id")); rawFeedID != "" {
		feedID, err := strconv.ParseUint(rawFeedID, 10, 64)
		if err != nil {
			return ItemFilters{}, err
		}
		filters.FeedID = uint(feedID)
	}
	for _, part := range []struct {
		raw    string
		target *string
	}{
		{raw: strings.TrimSpace(r.URL.Query().Get("from")), target: &filters.From},
		{raw: strings.TrimSpace(r.URL.Query().Get("to")), target: &filters.To},
	} {
		if part.raw == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", part.raw); err != nil {
			return ItemFilters{}, err
		}
		*part.target = part.raw
	}

	var feeds []models.Feed
	if err := db.Order("name ASC, id ASC").Find(&feeds).Error; err != nil {
		return ItemFilters{}, err
	}
	filters.FeedNames = make([]FeedOption, 0, len(feeds))
	for _, feed := range feeds {
		filters.FeedNames = append(filters.FeedNames, FeedOption{ID: feed.ID, Name: feed.Name})
	}

	return filters, nil
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

func normalizeItemSort(raw string) string {
	sort := itemSortOption(raw)
	switch sort {
	case itemSortTimeDesc, itemSortTimeAsc, itemSortShowsDesc, itemSortShowsAsc, itemSortReadsDesc, itemSortReadsAsc, itemSortRatingsDesc, itemSortRatingsAsc:
		return string(sort)
	default:
		return string(itemSortTimeDesc)
	}
}

func parsePage(r *http.Request) int {
	page, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("page")))
	if err != nil || page < 1 {
		return 1
	}
	return page
}

func buildPageURL(r *http.Request, page int) string {
	query := r.URL.Query()
	query.Set("page", strconv.Itoa(page))
	encoded := query.Encode()
	if encoded == "" {
		return r.URL.Path
	}
	return r.URL.Path + "?" + encoded
}

func paginateSlice[T any](items []T, page, size int, pageURL func(int) string) ([]T, Pagination) {
	total := len(items)
	if size <= 0 {
		size = pageSize
	}
	totalPages := total / size
	if total%size != 0 {
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
	start := (page - 1) * size
	end := start + size
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
		PageSize:   size,
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

func (h *Handler) queryShowRecords(db *gorm.DB, scope func(*gorm.DB) *gorm.DB, limit int) ([]ShowRecordView, error) {
	query := db.Table("item_shows").
		Select("item_shows.created_at, item_shows.device_id, item_shows.item_id, items.title as item_title, items.url as item_url, feeds.name as feed_name").
		Joins("JOIN items ON items.id = item_shows.item_id").
		Joins("LEFT JOIN feeds ON feeds.id = items.feed_id").
		Order("item_shows.created_at DESC")
	if scope != nil {
		query = scope(query)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}

	var records []ShowRecordView
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}

	return records, nil
}

func (h *Handler) queryReadRecords(db *gorm.DB, scope func(*gorm.DB) *gorm.DB, limit int) ([]ReadRecordView, error) {
	query := db.Table("item_reads").
		Select("item_reads.created_at, item_reads.device_id, item_reads.item_id, items.title as item_title, items.url as item_url, feeds.name as feed_name").
		Joins("JOIN items ON items.id = item_reads.item_id").
		Joins("LEFT JOIN feeds ON feeds.id = items.feed_id").
		Order("item_reads.created_at DESC")
	if scope != nil {
		query = scope(query)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}

	var records []ReadRecordView
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}

	return records, nil
}

func (h *Handler) queryRatingRecords(db *gorm.DB, scope func(*gorm.DB) *gorm.DB, limit int) ([]RatingRecordView, error) {
	query := db.Table("item_ratings").
		Select("item_ratings.created_at, item_ratings.device_id, item_ratings.item_id, item_ratings.rating, items.title as item_title, feeds.name as feed_name").
		Joins("JOIN items ON items.id = item_ratings.item_id").
		Joins("LEFT JOIN feeds ON feeds.id = items.feed_id").
		Order("item_ratings.created_at DESC")
	if scope != nil {
		query = scope(query)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}

	var records []RatingRecordView
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}

	return records, nil
}

type itemRatingAggregate struct {
	Count   int64
	Average float64
}

type countByItemRow struct {
	ItemID uint  `gorm:"column:item_id"`
	Count  int64 `gorm:"column:count"`
}

type ratingByItemRow struct {
	ItemID        uint    `gorm:"column:item_id"`
	RatingCount   int64   `gorm:"column:rating_count"`
	AverageRating float64 `gorm:"column:average_rating"`
}

type countByDeviceRow struct {
	DeviceID string `gorm:"column:device_id"`
	Count    int64  `gorm:"column:count"`
}

type deviceActivityAggregate struct {
	Count  int64
	LastAt time.Time
}

func loadFeedsByID(db *gorm.DB, ids []uint) (map[uint]models.Feed, error) {
	feedsByID := map[uint]models.Feed{}
	if len(ids) == 0 {
		return feedsByID, nil
	}

	var feeds []models.Feed
	if err := db.Where("id IN ?", ids).Find(&feeds).Error; err != nil {
		return nil, err
	}
	for _, feed := range feeds {
		feedsByID[feed.ID] = feed
	}

	return feedsByID, nil
}

func loadItemsByID(db *gorm.DB, ids []uint) (map[uint]models.Item, error) {
	itemsByID := map[uint]models.Item{}
	if len(ids) == 0 {
		return itemsByID, nil
	}

	var items []models.Item
	if err := db.Where("id IN ?", ids).Find(&items).Error; err != nil {
		return nil, err
	}
	for _, item := range items {
		itemsByID[item.ID] = item
	}

	return itemsByID, nil
}

func loadItemShowCounts(db *gorm.DB, itemIDs []uint) (map[uint]int64, error) {
	return loadItemCounts(db, &models.ItemShow{}, itemIDs)
}

func loadItemReadCounts(db *gorm.DB, itemIDs []uint) (map[uint]int64, error) {
	return loadItemCounts(db, &models.ItemRead{}, itemIDs)
}

func loadItemCounts(db *gorm.DB, model any, itemIDs []uint) (map[uint]int64, error) {
	counts := map[uint]int64{}
	if len(itemIDs) == 0 {
		return counts, nil
	}

	var rows []countByItemRow
	if err := db.Model(model).
		Select("item_id, COUNT(*) AS count").
		Where("item_id IN ?", itemIDs).
		Group("item_id").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.ItemID] = row.Count
	}

	return counts, nil
}

func loadItemRatingAggregates(db *gorm.DB, itemIDs []uint) (map[uint]itemRatingAggregate, error) {
	aggs := map[uint]itemRatingAggregate{}
	if len(itemIDs) == 0 {
		return aggs, nil
	}

	var rows []ratingByItemRow
	if err := db.Model(&models.ItemRating{}).
		Select("item_id, COUNT(*) AS rating_count, AVG(rating) AS average_rating").
		Where("item_id IN ?", itemIDs).
		Group("item_id").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		aggs[row.ItemID] = itemRatingAggregate{Count: row.RatingCount, Average: row.AverageRating}
	}

	return aggs, nil
}

func loadDeviceShowAggregates(db *gorm.DB) (map[string]deviceActivityAggregate, error) {
	return loadDeviceAggregates(db, &models.ItemShow{})
}

func loadDeviceReadAggregates(db *gorm.DB) (map[string]deviceActivityAggregate, error) {
	return loadDeviceAggregates(db, &models.ItemRead{})
}

func loadDeviceAggregates(db *gorm.DB, model any) (map[string]deviceActivityAggregate, error) {
	aggs := map[string]deviceActivityAggregate{}
	var rows []countByDeviceRow
	if err := db.Model(model).
		Select("device_id, COUNT(*) AS count").
		Group("device_id").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		agg := deviceActivityAggregate{Count: row.Count}
		switch typed := model.(type) {
		case *models.ItemShow:
			var last models.ItemShow
			if err := db.Where("device_id = ?", row.DeviceID).Order("created_at DESC, id DESC").First(&last).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			} else if err == nil {
				agg.LastAt = last.CreatedAt
			}
			_ = typed
		case *models.ItemRead:
			var last models.ItemRead
			if err := db.Where("device_id = ?", row.DeviceID).Order("created_at DESC, id DESC").First(&last).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			} else if err == nil {
				agg.LastAt = last.CreatedAt
			}
		}
		aggs[row.DeviceID] = agg
	}

	return aggs, nil
}

func loadDeviceRatingAggregates(db *gorm.DB) (map[string]deviceActivityAggregate, error) {
	aggs := map[string]deviceActivityAggregate{}
	var rows []countByDeviceRow
	if err := db.Model(&models.ItemRating{}).
		Select("device_id, COUNT(*) AS count").
		Group("device_id").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		agg := deviceActivityAggregate{Count: row.Count}
		var last models.ItemRating
		if err := db.Where("device_id = ?", row.DeviceID).Order("created_at DESC, id DESC").First(&last).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		} else if err == nil {
			agg.LastAt = last.CreatedAt
		}
		aggs[row.DeviceID] = agg
	}

	return aggs, nil
}
