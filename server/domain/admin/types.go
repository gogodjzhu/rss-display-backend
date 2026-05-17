package admin

import (
	"context"
	"time"

	"gorm.io/gorm"
)

var ErrNotFound = gorm.ErrRecordNotFound

type DashboardSummary struct {
	TotalFeeds   int64
	TotalItems   int64
	TotalDevices int64
	TotalShows   int64
	TotalReads   int64
	TotalRatings int64
}

type FeedSummary struct {
	ID          uint
	Name        string
	URL         string
	Enabled     bool
	CreatedAt   time.Time
	ItemCount   int64
	ShowCount   int64
	ReadCount   int64
	RatingCount int64
	AverageRating float64
	LastItemAt  *time.Time
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
	CurrentItemID    *uint
	CurrentItemTitle string
	HasCurrentItem   bool
	CurrentItemValue string
	LastSeen         time.Time
	CreatedAt        time.Time
	ShowCount        int64
	ReadCount        int64
	RatingCount      int64
	LastShowAt       *time.Time
	LastReadAt       *time.Time
	LastRatingAt     *time.Time
}

type ShowRecord struct {
	CreatedAt time.Time
	DeviceID  string
	ItemID    uint
	ItemTitle string
	ItemURL   string
	FeedName  string
}

type ReadRecord struct {
	CreatedAt time.Time
	DeviceID  string
	ItemID    uint
	ItemTitle string
	ItemURL   string
	FeedName  string
}

type RatingRecord struct {
	CreatedAt time.Time
	DeviceID  string
	ItemID    uint
	ItemTitle string
	FeedName  string
	Rating    int
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

type DashboardView struct {
	Summary       DashboardSummary
	RecentItems   []ItemSummary
	RecentShows   []ShowRecord
	RecentReads   []ReadRecord
	RecentRatings []RatingRecord
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

type ItemListView struct {
	Items      []ItemSummary
	Filters    ItemFilters
	Pagination Pagination
}

type ItemDetailView struct {
	Item    ItemSummary
	Shows   []ShowRecord
	Reads   []ReadRecord
	Ratings []RatingRecord
}

type DeviceListView struct {
	Devices    []DeviceSummary
	Pagination Pagination
}

type DeviceDetailView struct {
	Device  DeviceSummary
	Shows   []ShowRecord
	Reads   []ReadRecord
	Ratings []RatingRecord
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

type ReadModelService interface {
	Dashboard(ctx context.Context) (DashboardView, error)
	ListFeeds(ctx context.Context, page, pageSize int) (FeedListView, error)
	FeedDetail(ctx context.Context, feedID uint, page, pageSize int) (FeedDetailView, error)
	ListItems(ctx context.Context, filters ItemFilters, page, pageSize int) (ItemListView, error)
	ItemDetail(ctx context.Context, itemID uint) (ItemDetailView, error)
	ListDevices(ctx context.Context, page, pageSize int) (DeviceListView, error)
	DeviceDetail(ctx context.Context, deviceID string) (DeviceDetailView, error)
	FeedOptions(ctx context.Context) ([]FeedOption, error)
}