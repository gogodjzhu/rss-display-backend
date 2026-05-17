package steps

import (
	"github.com/esp32-rss-display/backend/server/pipeline"
	"gorm.io/gorm"
)

// PipelineName is the registered name of the RSS curation pipeline.
const PipelineName = "rss_pipeline"

// BuildRSSPipeline constructs the five-step RSS curation pipeline.
// db and runner are injected so each step can access the database and Python script.
// rateLimitMin and rateLimitMax (seconds) are passed through to the CrawlStep.
func BuildRSSPipeline(db *gorm.DB, runner *pipeline.PythonRunner, rateLimitMin, rateLimitMax int) *pipeline.Pipeline {
	return pipeline.New(
		PipelineName,
		NewFilterL1Step(db, runner),
		NewCrawlStep(db, runner, rateLimitMin, rateLimitMax),
		NewSummarizeStep(db, runner),
		NewFilterL2Step(db, runner),
		NewComposeStep(db, runner),
	)
}
