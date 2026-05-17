package steps

import (
	"context"
	"time"

	"github.com/esp32-rss-display/backend/server/models"
)

// DeviceGetter provides device access for pipeline steps.
type DeviceGetter interface {
	GetOrCreate(ctx context.Context, deviceID string) (*models.Device, error)
}

// ItemRanger provides item lookup by time range for pipeline steps.
type ItemRanger interface {
	FindByTimeRange(ctx context.Context, start, end time.Time) ([]models.Item, error)
}

// ItemFinder provides item lookup by IDs for pipeline steps.
type ItemFinder interface {
	FindByIDs(ctx context.Context, ids []uint) ([]models.Item, error)
}

// ItemUpdater provides item mutation for pipeline steps.
type ItemUpdater interface {
	UpdateContent(ctx context.Context, id uint, content string) error
	UpdateAbstract(ctx context.Context, id uint, abstract string) error
}

// JobReporter updates a job's report and level2 IDs.
type JobReporter interface {
	UpdateReport(ctx context.Context, jobID uint, report string, level2IDs []uint) error
}