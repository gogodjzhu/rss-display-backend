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

type Service interface {
	InitFeeds(ctx context.Context, configs []config.FeedConfig) error
	ListEnabled(ctx context.Context) ([]models.Feed, error)
	FindByID(ctx context.Context, id uint) (*models.Feed, error)
}

type serviceImpl struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &serviceImpl{repo: repo}
}

func (s *serviceImpl) InitFeeds(ctx context.Context, configs []config.FeedConfig) error {
	configuredURLs := make(map[string]struct{}, len(configs))

	for _, f := range configs {
		configuredURLs[f.URL] = struct{}{}

		existing, err := s.repo.FindByURL(ctx, f.URL)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := s.repo.Create(ctx, &models.Feed{
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
		if err := s.repo.Update(ctx, existing); err != nil {
			feedLog.Printf("failed to update feed %q: %v", f.Name, err)
		}
	}

	all, err := s.repo.FindAll(ctx)
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
		if err := s.repo.Update(ctx, feed); err != nil {
			feedLog.Printf("failed to disable stale feed %q: %v", feed.URL, err)
		}
	}
	return nil
}

func (s *serviceImpl) ListEnabled(ctx context.Context) ([]models.Feed, error) {
	all, err := s.repo.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	enabled := make([]models.Feed, 0, len(all))
	for i := range all {
		if all[i].Enabled {
			enabled = append(enabled, all[i])
		}
	}
	return enabled, nil
}

func (s *serviceImpl) FindByID(ctx context.Context, id uint) (*models.Feed, error) {
	return s.repo.FindByID(ctx, id)
}