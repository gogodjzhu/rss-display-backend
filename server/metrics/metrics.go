package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	RSSFetchTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rss_fetch_total",
		Help: "Total number of RSS feed fetch attempts",
	})

	RSSFetchError = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rss_fetch_error_total",
		Help: "Total number of RSS feed fetch errors",
	})

	RSSItemsParsedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rss_items_parsed_total",
		Help: "Total number of RSS items parsed and stored",
	})

	DeviceNextRequestTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "device_next_request_total",
		Help: "Total number of device next item requests",
	})

	DeviceRegisteredTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "device_registered_total",
		Help: "Total number of registered devices",
	})

	ImageRenderTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "image_render_total",
		Help: "Total number of item image render requests",
	})

	ImageDownloadFailureTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "image_download_failure_total",
		Help: "Total number of upstream image download or decode failures",
	})

	ImageColorCardTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "image_color_card_total",
		Help: "Total number of generated color-card image responses",
	})
)
