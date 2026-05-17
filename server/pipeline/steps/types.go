package steps

import "time"

// RSSJobInput is the JSON-encoded input written to the pipeline state under "job_input".
// It is serialised by the HTTP handler and deserialised by each step that needs it.
type RSSJobInput struct {
	DeviceID       string    `json:"device_id"`
	TimeRangeStart time.Time `json:"time_range_start"`
	TimeRangeEnd   time.Time `json:"time_range_end"`
}

// FilterL1Output is written to state under "filter_l1" by FilterL1Step.
type FilterL1Output struct {
	Level1IDs []uint `json:"level1_ids"`
}

// CrawlOutput is written to state under "crawl" by CrawlStep.
type CrawlOutput struct {
	CrawledIDs []uint `json:"crawled_ids"`
}

// SummarizeOutput is written to state under "summarize" by SummarizeStep.
type SummarizeOutput struct {
	SummarizedIDs []uint `json:"summarized_ids"`
}

// FilterL2Output is written to state under "filter_l2" by FilterL2Step.
type FilterL2Output struct {
	Level2IDs []uint `json:"level2_ids"`
}

// ComposeOutput is written to state under "compose" by ComposeStep.
type ComposeOutput struct {
	Report string `json:"report"`
}
