package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/esp32-rss-display/backend/server/models"
	"gorm.io/gorm"
)

type GORMReadModel struct {
	db *gorm.DB
}

func NewGORMReadModel(db *gorm.DB) *GORMReadModel {
	return &GORMReadModel{db: db}
}

func (rm *GORMReadModel) Dashboard(ctx context.Context) (DashboardView, error) {
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
		if err := rm.db.WithContext(ctx).Model(query.model).Count(query.target).Error; err != nil {
			return DashboardView{}, err
		}
	}

	recentItems, err := rm.listItemSummaries(ctx)
	if err != nil {
		return DashboardView{}, err
	}
	if len(recentItems) > 5 {
		recentItems = recentItems[:5]
	}
	recentShows, err := rm.queryShowRecords(ctx, nil, 5)
	if err != nil {
		return DashboardView{}, err
	}
	recentReads, err := rm.queryReadRecords(ctx, nil, 5)
	if err != nil {
		return DashboardView{}, err
	}
	recentRatings, err := rm.queryRatingRecords(ctx, nil, 5)
	if err != nil {
		return DashboardView{}, err
	}
	recentDevices, err := rm.listDeviceSummaries(ctx)
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

func (rm *GORMReadModel) ListFeeds(ctx context.Context, page, pageSize int) (FeedListView, error) {
	feeds, err := rm.listFeedSummaries(ctx)
	if err != nil {
		return FeedListView{}, err
	}
	paged, pagination := paginateSlice(feeds, page, pageSize, func(p int) string { return fmt.Sprintf("?page=%d", p) })
	return FeedListView{Feeds: paged, Pagination: pagination}, nil
}

func (rm *GORMReadModel) FeedDetail(ctx context.Context, feedID uint, page, pageSize int) (FeedDetailView, error) {
	var feed models.Feed
	if err := rm.db.WithContext(ctx).First(&feed, feedID).Error; err != nil {
		return FeedDetailView{}, err
	}

	items, err := rm.listItemSummaries(ctx)
	if err != nil {
		return FeedDetailView{}, err
	}
	filtered := make([]ItemSummary, 0)
	for _, item := range items {
		if item.FeedID == feed.ID {
			filtered = append(filtered, item)
		}
	}

	paged, pagination := paginateSlice(filtered, page, pageSize, func(p int) string { return fmt.Sprintf("?page=%d", p) })
	return FeedDetailView{
		Feed:       summarizeFeed(feed, filtered),
		Items:      paged,
		Pagination: pagination,
	}, nil
}

func (rm *GORMReadModel) ListItems(ctx context.Context, filters ItemFilters, page, pageSize int) (ItemListView, error) {
	items, err := rm.listItemSummaries(ctx)
	if err != nil {
		return ItemListView{}, err
	}
	items = applyItemFilters(items, filters)
	applyItemSort(items, itemSortOption(filters.Sort))
	paged, pagination := paginateSlice(items, page, pageSize, func(p int) string { return fmt.Sprintf("?page=%d", p) })
	return ItemListView{Items: paged, Filters: filters, Pagination: pagination}, nil
}

func (rm *GORMReadModel) ItemDetail(ctx context.Context, itemID uint) (ItemDetailView, error) {
	items, err := rm.listItemSummaries(ctx)
	if err != nil {
		return ItemDetailView{}, err
	}
	var selected *ItemSummary
	for i := range items {
		if items[i].ID == itemID {
			selected = &items[i]
			break
		}
	}
	if selected == nil {
		return ItemDetailView{}, gorm.ErrRecordNotFound
	}

	shows, err := rm.queryShowRecords(ctx, func(query *gorm.DB) *gorm.DB {
		return query.Where("item_shows.item_id = ?", itemID)
	}, 0)
	if err != nil {
		return ItemDetailView{}, err
	}
	reads, err := rm.queryReadRecords(ctx, func(query *gorm.DB) *gorm.DB {
		return query.Where("item_reads.item_id = ?", itemID)
	}, 0)
	if err != nil {
		return ItemDetailView{}, err
	}
	ratings, err := rm.queryRatingRecords(ctx, func(query *gorm.DB) *gorm.DB {
		return query.Where("item_ratings.item_id = ?", itemID)
	}, 0)
	if err != nil {
		return ItemDetailView{}, err
	}

	return ItemDetailView{
		Item:    *selected,
		Shows:   shows,
		Reads:   reads,
		Ratings: ratings,
	}, nil
}

func (rm *GORMReadModel) ListDevices(ctx context.Context, page, pageSize int) (DeviceListView, error) {
	devices, err := rm.listDeviceSummaries(ctx)
	if err != nil {
		return DeviceListView{}, err
	}
	paged, pagination := paginateSlice(devices, page, pageSize, func(p int) string { return fmt.Sprintf("?page=%d", p) })
	return DeviceListView{Devices: paged, Pagination: pagination}, nil
}

func (rm *GORMReadModel) DeviceDetail(ctx context.Context, deviceID string) (DeviceDetailView, error) {
	devices, err := rm.listDeviceSummaries(ctx)
	if err != nil {
		return DeviceDetailView{}, err
	}
	var selected *DeviceSummary
	for i := range devices {
		if devices[i].DeviceID == deviceID {
			selected = &devices[i]
			break
		}
	}
	if selected == nil {
		return DeviceDetailView{}, gorm.ErrRecordNotFound
	}

	shows, err := rm.queryShowRecords(ctx, func(query *gorm.DB) *gorm.DB {
		return query.Where("item_shows.device_id = ?", deviceID)
	}, 0)
	if err != nil {
		return DeviceDetailView{}, err
	}
	reads, err := rm.queryReadRecords(ctx, func(query *gorm.DB) *gorm.DB {
		return query.Where("item_reads.device_id = ?", deviceID)
	}, 0)
	if err != nil {
		return DeviceDetailView{}, err
	}
	ratings, err := rm.queryRatingRecords(ctx, func(query *gorm.DB) *gorm.DB {
		return query.Where("item_ratings.device_id = ?", deviceID)
	}, 0)
	if err != nil {
		return DeviceDetailView{}, err
	}

	return DeviceDetailView{
		Device:  *selected,
		Shows:   shows,
		Reads:   reads,
		Ratings: ratings,
	}, nil
}

func (rm *GORMReadModel) FeedOptions(ctx context.Context) ([]FeedOption, error) {
	var feeds []models.Feed
	if err := rm.db.WithContext(ctx).Order("name ASC, id ASC").Find(&feeds).Error; err != nil {
		return nil, err
	}
	options := make([]FeedOption, 0, len(feeds))
	for _, feed := range feeds {
		options = append(options, FeedOption{ID: feed.ID, Name: feed.Name})
	}
	return options, nil
}

func (rm *GORMReadModel) listItemSummaries(ctx context.Context) ([]ItemSummary, error) {
	var items []models.Item
	if err := rm.db.WithContext(ctx).Model(&models.Item{}).Order("COALESCE(published_at, created_at) DESC, id DESC").Find(&items).Error; err != nil {
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

	feedsByID, err := rm.loadFeedsByID(ctx, feedIDs)
	if err != nil {
		return nil, err
	}
	showCounts, err := rm.loadItemCounts(ctx, &models.ItemShow{}, itemIDs)
	if err != nil {
		return nil, err
	}
	readCounts, err := rm.loadItemCounts(ctx, &models.ItemRead{}, itemIDs)
	if err != nil {
		return nil, err
	}
	ratingAggs, err := rm.loadItemRatingAggregates(ctx, itemIDs)
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
			FeedName:       feedsByID[item.FeedID].Name,
			Title:         item.Title,
			URL:           item.URL,
			PublishedAt:   item.PublishedAt,
			CreatedAt:     item.CreatedAt,
			DisplayTime:   itemTime,
			ShowCount:     showCounts[item.ID],
			ReadCount:     readCounts[item.ID],
			RatingCount:   agg.RatingCount,
			AverageRating: agg.AverageRating,
		})
	}
	return summaries, nil
}

func (rm *GORMReadModel) listFeedSummaries(ctx context.Context) ([]FeedSummary, error) {
	var feeds []models.Feed
	if err := rm.db.WithContext(ctx).Order("name ASC, id ASC").Find(&feeds).Error; err != nil {
		return nil, err
	}

	items, err := rm.listItemSummaries(ctx)
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

func (rm *GORMReadModel) listDeviceSummaries(ctx context.Context) ([]DeviceSummary, error) {
	var devices []models.Device
	if err := rm.db.WithContext(ctx).Order("last_seen DESC, device_id ASC").Find(&devices).Error; err != nil {
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

	itemsByID, err := rm.loadItemsByID(ctx, currentItemIDs)
	if err != nil {
		return nil, err
	}
	showAggs, err := rm.loadDeviceActivityAggregates(ctx, &models.ItemShow{})
	if err != nil {
		return nil, err
	}
	readAggs, err := rm.loadDeviceActivityAggregates(ctx, &models.ItemRead{})
	if err != nil {
		return nil, err
	}
	ratingAggs, err := rm.loadDeviceRatingAggregates(ctx)
	if err != nil {
		return nil, err
	}

	summaries := make([]DeviceSummary, 0, len(devices))
	for _, device := range devices {
		showAgg := showAggs[device.DeviceID]
		readAgg := readAggs[device.DeviceID]
		ratingAgg := ratingAggs[device.DeviceID]
		summary := DeviceSummary{
			DeviceID:         device.DeviceID,
			CurrentItemID:    device.CurrentItemID,
			LastSeen:         device.LastSeen,
			CreatedAt:        device.CreatedAt,
			ShowCount:        showAgg.Count,
			ReadCount:        readAgg.Count,
			RatingCount:      ratingAgg.Count,
			HasCurrentItem:   device.CurrentItemID != nil,
			CurrentItemValue: fmt.Sprintf("%d", func() uint {
				if device.CurrentItemID != nil {
					return *device.CurrentItemID
				}
				return 0
			}()),
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
			summary.CurrentItemTitle = itemsByID[*device.CurrentItemID].Title
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

func (rm *GORMReadModel) queryShowRecords(ctx context.Context, scope func(*gorm.DB) *gorm.DB, limit int) ([]ShowRecord, error) {
	query := rm.db.WithContext(ctx).Table("item_shows").
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
	var records []ShowRecord
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (rm *GORMReadModel) queryReadRecords(ctx context.Context, scope func(*gorm.DB) *gorm.DB, limit int) ([]ReadRecord, error) {
	query := rm.db.WithContext(ctx).Table("item_reads").
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
	var records []ReadRecord
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (rm *GORMReadModel) queryRatingRecords(ctx context.Context, scope func(*gorm.DB) *gorm.DB, limit int) ([]RatingRecord, error) {
	query := rm.db.WithContext(ctx).Table("item_ratings").
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
	var records []RatingRecord
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

type itemRatingAggregate struct {
	RatingCount   int64
	AverageRating float64
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

func (rm *GORMReadModel) loadFeedsByID(ctx context.Context, ids []uint) (map[uint]models.Feed, error) {
	feedsByID := map[uint]models.Feed{}
	if len(ids) == 0 {
		return feedsByID, nil
	}
	var feeds []models.Feed
	if err := rm.db.WithContext(ctx).Where("id IN ?", ids).Find(&feeds).Error; err != nil {
		return nil, err
	}
	for _, feed := range feeds {
		feedsByID[feed.ID] = feed
	}
	return feedsByID, nil
}

func (rm *GORMReadModel) loadItemsByID(ctx context.Context, ids []uint) (map[uint]models.Item, error) {
	itemsByID := map[uint]models.Item{}
	if len(ids) == 0 {
		return itemsByID, nil
	}
	var items []models.Item
	if err := rm.db.WithContext(ctx).Where("id IN ?", ids).Find(&items).Error; err != nil {
		return nil, err
	}
	for _, item := range items {
		itemsByID[item.ID] = item
	}
	return itemsByID, nil
}

func (rm *GORMReadModel) loadItemCounts(ctx context.Context, model any, itemIDs []uint) (map[uint]int64, error) {
	counts := map[uint]int64{}
	if len(itemIDs) == 0 {
		return counts, nil
	}
	var rows []countByItemRow
	if err := rm.db.WithContext(ctx).Model(model).
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

func (rm *GORMReadModel) loadItemRatingAggregates(ctx context.Context, itemIDs []uint) (map[uint]itemRatingAggregate, error) {
	aggs := map[uint]itemRatingAggregate{}
	if len(itemIDs) == 0 {
		return aggs, nil
	}
	var rows []ratingByItemRow
	if err := rm.db.WithContext(ctx).Model(&models.ItemRating{}).
		Select("item_id, COUNT(*) AS rating_count, AVG(rating) AS average_rating").
		Where("item_id IN ?", itemIDs).
		Group("item_id").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		aggs[row.ItemID] = itemRatingAggregate{RatingCount: row.RatingCount, AverageRating: row.AverageRating}
	}
	return aggs, nil
}

func (rm *GORMReadModel) loadDeviceActivityAggregates(ctx context.Context, model any) (map[string]deviceActivityAggregate, error) {
	aggs := map[string]deviceActivityAggregate{}
	var rows []countByDeviceRow
	if err := rm.db.WithContext(ctx).Model(model).
		Select("device_id, COUNT(*) AS count").
		Group("device_id").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		agg := deviceActivityAggregate{Count: row.Count}
		switch m := model.(type) {
		case *models.ItemShow:
			var last models.ItemShow
			if err := rm.db.WithContext(ctx).Where("device_id = ?", row.DeviceID).Order("created_at DESC, id DESC").First(&last).Error; err == nil {
				agg.LastAt = last.CreatedAt
			}
			_ = m
		case *models.ItemRead:
			var last models.ItemRead
			if err := rm.db.WithContext(ctx).Where("device_id = ?", row.DeviceID).Order("created_at DESC, id DESC").First(&last).Error; err == nil {
				agg.LastAt = last.CreatedAt
			}
		}
		aggs[row.DeviceID] = agg
	}
	return aggs, nil
}

func (rm *GORMReadModel) loadDeviceRatingAggregates(ctx context.Context) (map[string]deviceActivityAggregate, error) {
	aggs := map[string]deviceActivityAggregate{}
	var rows []countByDeviceRow
	if err := rm.db.WithContext(ctx).Model(&models.ItemRating{}).
		Select("device_id, COUNT(*) AS count").
		Group("device_id").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		agg := deviceActivityAggregate{Count: row.Count}
		var last models.ItemRating
		if err := rm.db.WithContext(ctx).Where("device_id = ?", row.DeviceID).Order("created_at DESC, id DESC").First(&last).Error; err == nil {
			agg.LastAt = last.CreatedAt
		}
		aggs[row.DeviceID] = agg
	}
	return aggs, nil
}