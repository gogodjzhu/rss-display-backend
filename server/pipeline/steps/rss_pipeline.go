package steps

import (
	"github.com/esp32-rss-display/backend/server/pipeline"
)

// PipelineName is the registered name of the RSS curation pipeline.
const PipelineName = "rss_pipeline"

// BuildRSSPipeline constructs the five-step RSS curation pipeline.
// Domain services are injected so each step can access data without direct DB coupling.
func BuildRSSPipeline(
	devices DeviceGetter,
	itemsFinder ItemFinder,
	itemsRanger ItemRanger,
	itemUpdater ItemUpdater,
	jobReporter JobReporter,
	runner *pipeline.PythonRunner,
	rateLimitMin, rateLimitMax int,
) *pipeline.Pipeline {
	return pipeline.New(
		PipelineName,
		NewFilterL1Step(devices, itemsRanger, runner),
		NewCrawlStep(itemsFinder, itemUpdater, runner, rateLimitMin, rateLimitMax),
		NewSummarizeStep(itemsFinder, itemUpdater, runner),
		NewFilterL2Step(devices, itemsFinder, runner),
		NewComposeStep(devices, itemsFinder, jobReporter, runner),
	)
}