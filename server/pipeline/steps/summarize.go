package steps

import (
	"context"
	"fmt"
	"time"

	"github.com/esp32-rss-display/backend/server/models"
	"github.com/esp32-rss-display/backend/server/pipeline"
	"gorm.io/gorm"
)

// SummarizeStep generates an abstract for each crawled item and persists it
// to the database. It writes the list of summarised IDs to state under "summarize".
type SummarizeStep struct {
	db     *gorm.DB
	runner *pipeline.PythonRunner
}

// NewSummarizeStep constructs a SummarizeStep.
func NewSummarizeStep(db *gorm.DB, runner *pipeline.PythonRunner) *SummarizeStep {
	return &SummarizeStep{db: db, runner: runner}
}

func (s *SummarizeStep) Name() string { return "summarize" }

func (s *SummarizeStep) Config() pipeline.StepConfig {
	return pipeline.StepConfig{
		Timeout:     20 * time.Minute,
		RetryPolicy: pipeline.RetryPolicy{MaxAttempts: 2, BaseDelay: 5 * time.Second, MaxDelay: 60 * time.Second},
	}
}

func (s *SummarizeStep) Run(ctx context.Context, state pipeline.StateAccessor) error {
	crawl, err := pipeline.GetState[CrawlOutput](state, "crawl")
	if err != nil {
		return err
	}

	if len(crawl.CrawledIDs) == 0 {
		return pipeline.SetState(state, s.Name(), SummarizeOutput{SummarizedIDs: nil})
	}

	var items []models.Item
	if err := s.db.Where("id IN ? AND content != ''", crawl.CrawledIDs).Find(&items).Error; err != nil {
		return fmt.Errorf("summarize: get items: %w", err)
	}

	type itemEntry struct {
		ID      uint   `json:"id"`
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	entries := make([]itemEntry, len(items))
	for i, item := range items {
		entries[i] = itemEntry{ID: item.ID, Title: item.Title, Content: item.Content}
	}

	pyInput := map[string]any{"items": entries}

	inPath, outPath, cleanup := tempIOPaths("summarize")
	defer cleanup()

	if err := s.runner.WriteJSON(inPath, pyInput); err != nil {
		return fmt.Errorf("summarize: write input: %w", err)
	}
	if err := s.runner.RunCtx(ctx, "summarize", inPath, outPath); err != nil {
		return fmt.Errorf("summarize: python: %w", err)
	}

	var pyOutput struct {
		Results []struct {
			ID       uint   `json:"id"`
			Abstract string `json:"abstract"`
		} `json:"results"`
	}
	if err := s.runner.ReadJSON(outPath, &pyOutput); err != nil {
		return fmt.Errorf("summarize: read output: %w", err)
	}

	var summarizedIDs []uint
	for _, r := range pyOutput.Results {
		if r.Abstract != "" {
			if err := s.db.Model(&models.Item{}).Where("id = ?", r.ID).
				Update("abstract", r.Abstract).Error; err != nil {
				return fmt.Errorf("summarize: update item %d: %w", r.ID, err)
			}
		}
		summarizedIDs = append(summarizedIDs, r.ID)
	}

	return pipeline.SetState(state, s.Name(), SummarizeOutput{SummarizedIDs: summarizedIDs})
}
