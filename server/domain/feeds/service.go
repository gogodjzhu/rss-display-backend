package feeds

import (
	"context"
	"errors"

	"github.com/esp32-rss-display/backend/server/config"
	"github.com/esp32-rss-display/backend/server/logger"
	"github.com/esp32-rss-display/backend/server/models"
	"gorm.io/gorm"
)

var feedLog = logger.Get("feed")

// Service is the business-logic contract for feed management.
type Service interface {
	// InitFeeds synchronises the configured feeds list with the database.
	// It creates new feeds, updates names/enabled status, and disables feeds
	// that are no longer in the config.
	InitFeeds(ctx context.Context, db *gorm.DB, configs []config.FeedConfig) error
}

type serviceImpl struct {
	repo Repository
}

// NewService creates a Service backed by the given repository.
func NewService(repo Repository) Service {
	return &serviceImpl{repo: repo}
}

func (s *serviceImpl) InitFeeds(ctx context.Context, db *gorm.DB, configs []config.FeedConfig) error {
	configuredURLs := make(map[string]struct{}, len(configs))

	for _, f := range configs {
		configuredURLs[f.URL] = struct{}{}

		existing, err := s.repo.FindByURL(ctx, db, f.URL)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := s.repo.Create(ctx, db, &models.Feed{
				Name:    f.Name,
				URL:     f.URL,
				Enabled: f.Enabled,
			}); err != nil {
				feedLog.Printf("failed to create feed %q: %v", f.Name, err)
			}
			continue
		}
		if err != nil {
			feedLog.Printf("failed to look up feed %q: %v", f.URL, err)
			continue
		}
		existing.Name = f.Name
		existing.Enabled = f.Enabled
		if err := s.repo.Update(ctx, db, existing); err != nil {
			feedLog.Printf("failed to update feed %q: %v", f.Name, err)
		}
	}

	// Disable feeds no longer in config.
	all, err := s.repo.FindAll(ctx, db)
	if err != nil {
		return err
	}
	for i := range all {
		feed := &all[i]
		if _, ok := configuredURLs[feed.URL]; ok {
			continue
		}
		if !feed.Enabled {
			continue
		}
		feed.Enabled = false
		if err := s.repo.Update(ctx, db, feed); err != nil {
			feedLog.Printf("failed to disable stale feed %q: %v", feed.URL, err)
		}
	}
	return nil
}
