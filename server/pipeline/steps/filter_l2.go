package steps

import (
	"context"
	"fmt"
	"time"

	"github.com/esp32-rss-display/backend/server/pipeline"
)

// FilterL2Step applies a second, richer filter using item abstracts, narrowing the
// candidate set to items most relevant to the device owner's preferences.
// It writes the selected IDs to state under "filter_l2".
type FilterL2Step struct {
	devices DeviceGetter
	items   ItemFinder
	runner  *pipeline.PythonRunner
}

// NewFilterL2Step constructs a FilterL2Step.
func NewFilterL2Step(devices DeviceGetter, items ItemFinder, runner *pipeline.PythonRunner) *FilterL2Step {
	return &FilterL2Step{devices: devices, items: items, runner: runner}
}

func (s *FilterL2Step) Name() string { return "filter_l2" }

func (s *FilterL2Step) Config() pipeline.StepConfig {
	return pipeline.StepConfig{
		Timeout:     10 * time.Minute,
		RetryPolicy: pipeline.RetryPolicy{MaxAttempts: 2, BaseDelay: 5 * time.Second, MaxDelay: 30 * time.Second},
	}
}

func (s *FilterL2Step) Run(ctx context.Context, state pipeline.StateAccessor) error {
	input, err := pipeline.GetState[RSSJobInput](state, "job_input")
	if err != nil {
		return err
	}

	summarize, err := pipeline.GetState[SummarizeOutput](state, "summarize")
	if err != nil {
		return err
	}

	if len(summarize.SummarizedIDs) == 0 {
		return pipeline.SetState(state, s.Name(), FilterL2Output{Level2IDs: nil})
	}

	device, err := s.devices.GetOrCreate(ctx, input.DeviceID)
	if err != nil {
		return fmt.Errorf("filter_l2: get device: %w", err)
	}

	itemList, err := s.items.FindByIDs(ctx, summarize.SummarizedIDs)
	if err != nil {
		return fmt.Errorf("filter_l2: get items: %w", err)
	}

	var filteredItems []struct {
		ID       uint
		Title    string
		Abstract string
	}
	for _, item := range itemList {
		if item.Abstract != "" {
			filteredItems = append(filteredItems, struct {
				ID       uint
				Title    string
				Abstract string
			}{ID: item.ID, Title: item.Title, Abstract: item.Abstract})
		}
	}

	type itemEntry struct {
		ID       uint   `json:"id"`
		Title    string `json:"title"`
		Abstract string `json:"abstract"`
	}
	entries := make([]itemEntry, len(filteredItems))
	for i, item := range filteredItems {
		entries[i] = itemEntry{ID: item.ID, Title: item.Title, Abstract: item.Abstract}
	}

	pyInput := map[string]any{
		"device": map[string]string{
			"role":       device.Role,
			"preference": device.Preference,
		},
		"items": entries,
	}

	inPath, outPath, cleanup := tempIOPaths("filter_l2")
	defer cleanup()

	if err := s.runner.WriteJSON(inPath, pyInput); err != nil {
		return fmt.Errorf("filter_l2: write input: %w", err)
	}
	if err := s.runner.RunCtx(ctx, "filter_l2", inPath, outPath); err != nil {
		return fmt.Errorf("filter_l2: python: %w", err)
	}

	var pyOutput struct {
		Level2IDs []uint `json:"level2_ids"`
	}
	if err := s.runner.ReadJSON(outPath, &pyOutput); err != nil {
		return fmt.Errorf("filter_l2: read output: %w", err)
	}

	return pipeline.SetState(state, s.Name(), FilterL2Output{Level2IDs: pyOutput.Level2IDs})
}