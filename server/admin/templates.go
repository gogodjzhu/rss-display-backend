package admin

import (
	"fmt"
	"html/template"
	"time"
)

func mustParseTemplates() *template.Template {
	funcs := template.FuncMap{
		"formatTime": func(value any) string {
			switch v := value.(type) {
			case time.Time:
				if v.IsZero() {
					return "-"
				}
				return v.UTC().Format("2006-01-02 15:04:05")
			case *time.Time:
				if v == nil || v.IsZero() {
					return "-"
				}
				return v.UTC().Format("2006-01-02 15:04:05")
			default:
				return "-"
			}
		},
		"formatFloat": func(v float64) string {
			return fmt.Sprintf("%.2f", v)
		},
	}

	return template.Must(template.New("layout").Funcs(funcs).Parse(layoutTemplate + paginationTemplate + dashboardTemplate + feedsListTemplate + feedDetailTemplate + itemsListTemplate + itemDetailTemplate + devicesListTemplate + deviceDetailTemplate))
}

const layoutTemplate = `
{{define "layout"}}
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}} | RSS Admin</title>
  <style>
    :root {
      color-scheme: light;
      font-family: Arial, Helvetica, sans-serif;
      background: #f4f6fb;
      color: #1f2937;
    }
    body {
      margin: 0;
      background: #f4f6fb;
      color: #1f2937;
    }
    header {
      background: #111827;
      color: #fff;
      padding: 16px 24px;
    }
    nav a {
      color: #cbd5e1;
      text-decoration: none;
      margin-right: 16px;
      font-weight: 600;
    }
    nav a.active, nav a:hover {
      color: #fff;
    }
    main {
      padding: 24px;
      max-width: 1280px;
      margin: 0 auto;
    }
    h1, h2 {
      margin-top: 0;
    }
    .cards {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
      gap: 16px;
      margin-bottom: 24px;
    }
    .card, .panel {
      background: #fff;
      border: 1px solid #dbe3f0;
      border-radius: 10px;
      padding: 16px;
      box-shadow: 0 1px 3px rgba(15, 23, 42, 0.06);
    }
    .card .label {
      color: #64748b;
      font-size: 14px;
      margin-bottom: 8px;
    }
    .card .value {
      font-size: 28px;
      font-weight: 700;
    }
    .grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
      gap: 16px;
    }
    table {
      width: 100%;
      border-collapse: collapse;
      background: #fff;
    }
    th, td {
      text-align: left;
      padding: 10px 12px;
      border-bottom: 1px solid #e5e7eb;
      vertical-align: top;
    }
    th {
      color: #475569;
      font-size: 13px;
      text-transform: uppercase;
      letter-spacing: 0.03em;
    }
    a {
      color: #2563eb;
    }
    .meta {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
      gap: 12px;
      margin-bottom: 24px;
    }
    .meta strong {
      display: block;
      color: #475569;
      font-size: 13px;
      margin-bottom: 4px;
    }
    .pill {
      display: inline-block;
      padding: 2px 8px;
      border-radius: 999px;
      font-size: 12px;
      font-weight: 700;
      background: #dbeafe;
      color: #1d4ed8;
    }
    .pill.off {
      background: #e5e7eb;
      color: #374151;
    }
    .empty {
      color: #64748b;
      font-style: italic;
    }
    .filters {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
      gap: 12px;
      margin-bottom: 16px;
      align-items: end;
    }
    .filters label {
      display: block;
      font-size: 13px;
      color: #475569;
      margin-bottom: 4px;
    }
    .filters input, .filters select, .filters button {
      width: 100%;
      box-sizing: border-box;
      padding: 8px 10px;
      border: 1px solid #cbd5e1;
      border-radius: 8px;
      font: inherit;
      background: #fff;
    }
    .filters .actions {
      display: flex;
      gap: 8px;
      align-items: center;
    }
    .filters .actions a {
      text-decoration: none;
      color: #475569;
      font-size: 14px;
    }
    .pagination {
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 12px;
      margin-top: 16px;
      color: #475569;
      font-size: 14px;
    }
    .pagination .links {
      display: flex;
      gap: 8px;
      align-items: center;
    }
    .pagination a {
      text-decoration: none;
      padding: 6px 10px;
      border: 1px solid #cbd5e1;
      border-radius: 8px;
      background: #fff;
    }
  </style>
</head>
<body>
  <header>
    <h1>RSS Admin</h1>
    <nav>
      <a href="/admin" class="{{if eq .ActivePath "/admin"}}active{{end}}">Dashboard</a>
      <a href="/admin/feeds" class="{{if eq .ActivePath "/admin/feeds"}}active{{end}}">Feeds</a>
      <a href="/admin/items" class="{{if eq .ActivePath "/admin/items"}}active{{end}}">Items</a>
      <a href="/admin/devices" class="{{if eq .ActivePath "/admin/devices"}}active{{end}}">Devices</a>
    </nav>
  </header>
  <main>
    {{.Content}}
  </main>
</body>
</html>
{{end}}
`

const paginationTemplate = `
{{define "pagination"}}
{{if gt .TotalItems 0}}
<div class="pagination">
  <div>Showing {{.Start}}-{{.End}} of {{.TotalItems}}</div>
  <div class="links">
    {{if .HasPrev}}<a href="{{.PrevURL}}">Previous</a>{{end}}
    <span>Page {{.Page}} / {{.TotalPages}}</span>
    {{if .HasNext}}<a href="{{.NextURL}}">Next</a>{{end}}
  </div>
</div>
{{end}}
{{end}}
`

const dashboardTemplate = `
{{define "dashboard"}}
<h1>Dashboard</h1>
<div class="cards">
  <div class="card"><div class="label">Feeds</div><div class="value">{{.Summary.TotalFeeds}}</div></div>
  <div class="card"><div class="label">Items</div><div class="value">{{.Summary.TotalItems}}</div></div>
  <div class="card"><div class="label">Devices</div><div class="value">{{.Summary.TotalDevices}}</div></div>
  <div class="card"><div class="label">Shows</div><div class="value">{{.Summary.TotalShows}}</div></div>
  <div class="card"><div class="label">Reads</div><div class="value">{{.Summary.TotalReads}}</div></div>
  <div class="card"><div class="label">Ratings</div><div class="value">{{.Summary.TotalRatings}}</div></div>
</div>
<div class="grid">
  <section class="panel">
    <h2>Recent Items</h2>
    {{if .RecentItems}}
    <table>
      <thead><tr><th>Item</th><th>Feed</th><th>Time</th></tr></thead>
      <tbody>
        {{range .RecentItems}}
        <tr>
          <td><a href="/admin/items/{{.ID}}">{{.Title}}</a></td>
          <td>{{.FeedName}}</td>
          <td>{{formatTime .DisplayTime}}</td>
        </tr>
        {{end}}
      </tbody>
    </table>
    {{else}}<p class="empty">No items yet.</p>{{end}}
  </section>
  <section class="panel">
    <h2>Recent Shows</h2>
    {{if .RecentShows}}
    <table>
      <thead><tr><th>Time</th><th>Device</th><th>Item</th></tr></thead>
      <tbody>
        {{range .RecentShows}}
        <tr>
          <td>{{formatTime .CreatedAt}}</td>
          <td><a href="/admin/devices/{{.DeviceID}}">{{.DeviceID}}</a></td>
          <td><a href="/admin/items/{{.ItemID}}">{{.ItemTitle}}</a></td>
        </tr>
        {{end}}
      </tbody>
    </table>
    {{else}}<p class="empty">No shows yet.</p>{{end}}
  </section>
  <section class="panel">
    <h2>Recent Reads</h2>
    {{if .RecentReads}}
    <table>
      <thead><tr><th>Time</th><th>Device</th><th>Item</th></tr></thead>
      <tbody>
        {{range .RecentReads}}
        <tr>
          <td>{{formatTime .CreatedAt}}</td>
          <td><a href="/admin/devices/{{.DeviceID}}">{{.DeviceID}}</a></td>
          <td><a href="/admin/items/{{.ItemID}}">{{.ItemTitle}}</a></td>
        </tr>
        {{end}}
      </tbody>
    </table>
    {{else}}<p class="empty">No reads yet.</p>{{end}}
  </section>
  <section class="panel">
    <h2>Recent Ratings</h2>
    {{if .RecentRatings}}
    <table>
      <thead><tr><th>Time</th><th>Device</th><th>Rating</th></tr></thead>
      <tbody>
        {{range .RecentRatings}}
        <tr>
          <td>{{formatTime .CreatedAt}}</td>
          <td><a href="/admin/devices/{{.DeviceID}}">{{.DeviceID}}</a></td>
          <td><a href="/admin/items/{{.ItemID}}">{{.ItemTitle}}</a> ({{.Rating}})</td>
        </tr>
        {{end}}
      </tbody>
    </table>
    {{else}}<p class="empty">No ratings yet.</p>{{end}}
  </section>
</div>
{{end}}
`

const feedsListTemplate = `
{{define "feeds_list"}}
<h1>Feeds</h1>
{{if .Feeds}}
<table>
  <thead>
    <tr><th>Name</th><th>Status</th><th>Items</th><th>Shows</th><th>Reads</th><th>Ratings</th><th>Avg Rating</th><th>Last Item</th></tr>
  </thead>
  <tbody>
    {{range .Feeds}}
    <tr>
      <td><a href="/admin/feeds/{{.ID}}">{{.Name}}</a><br><small>{{.URL}}</small></td>
      <td>{{if .Enabled}}<span class="pill">Enabled</span>{{else}}<span class="pill off">Disabled</span>{{end}}</td>
      <td>{{.ItemCount}}</td>
      <td>{{.ShowCount}}</td>
      <td>{{.ReadCount}}</td>
      <td>{{.RatingCount}}</td>
      <td>{{if gt .RatingCount 0}}{{formatFloat .AverageRating}}{{else}}-{{end}}</td>
      <td>{{formatTime .LastItemAt}}</td>
    </tr>
    {{end}}
  </tbody>
</table>
{{template "pagination" .Pagination}}
{{else}}<p class="empty">No feeds configured.</p>{{end}}
{{end}}
`

const feedDetailTemplate = `
{{define "feed_detail"}}
<h1>{{.Feed.Name}}</h1>
<div class="meta">
  <div class="card"><strong>URL</strong>{{.Feed.URL}}</div>
  <div class="card"><strong>Status</strong>{{if .Feed.Enabled}}Enabled{{else}}Disabled{{end}}</div>
  <div class="card"><strong>Items</strong>{{.Feed.ItemCount}}</div>
  <div class="card"><strong>Shows</strong>{{.Feed.ShowCount}}</div>
  <div class="card"><strong>Reads</strong>{{.Feed.ReadCount}}</div>
  <div class="card"><strong>Ratings</strong>{{.Feed.RatingCount}}</div>
  <div class="card"><strong>Average Rating</strong>{{if gt .Feed.RatingCount 0}}{{formatFloat .Feed.AverageRating}}{{else}}-{{end}}</div>
</div>
<section class="panel">
  <h2>Items</h2>
  {{if .Items}}
  <table>
    <thead><tr><th>Item</th><th>Time</th><th>Shows</th><th>Reads</th><th>Ratings</th><th>Avg Rating</th></tr></thead>
    <tbody>
      {{range .Items}}
      <tr>
        <td><a href="/admin/items/{{.ID}}">{{.Title}}</a></td>
        <td>{{formatTime .DisplayTime}}</td>
        <td>{{.ShowCount}}</td>
        <td>{{.ReadCount}}</td>
        <td>{{.RatingCount}}</td>
        <td>{{if gt .RatingCount 0}}{{formatFloat .AverageRating}}{{else}}-{{end}}</td>
      </tr>
      {{end}}
    </tbody>
  </table>
  {{template "pagination" .Pagination}}
  {{else}}<p class="empty">No items for this feed.</p>{{end}}
</section>
{{end}}
`

const itemsListTemplate = `
{{define "items_list"}}
<h1>Items</h1>
<section class="panel">
  <form method="get" action="/admin/items">
    <div class="filters">
      <div>
        <label for="title">Title</label>
        <input id="title" type="text" name="title" value="{{.Filters.Title}}">
      </div>
      <div>
        <label for="feed_id">Feed</label>
        <select id="feed_id" name="feed_id">
          <option value="">All feeds</option>
          {{range .Filters.FeedNames}}
          <option value="{{.ID}}" {{if eq $.Filters.FeedID .ID}}selected{{end}}>{{.Name}}</option>
          {{end}}
        </select>
      </div>
      <div>
        <label for="from">From</label>
        <input id="from" type="date" name="from" value="{{.Filters.From}}">
      </div>
      <div>
        <label for="to">To</label>
        <input id="to" type="date" name="to" value="{{.Filters.To}}">
      </div>
      <div>
        <label for="sort">Sort</label>
        <select id="sort" name="sort">
          <option value="time_desc" {{if eq .Filters.Sort "time_desc"}}selected{{end}}>Time desc</option>
          <option value="time_asc" {{if eq .Filters.Sort "time_asc"}}selected{{end}}>Time asc</option>
          <option value="shows_desc" {{if eq .Filters.Sort "shows_desc"}}selected{{end}}>Shows desc</option>
          <option value="shows_asc" {{if eq .Filters.Sort "shows_asc"}}selected{{end}}>Shows asc</option>
          <option value="reads_desc" {{if eq .Filters.Sort "reads_desc"}}selected{{end}}>Reads desc</option>
          <option value="reads_asc" {{if eq .Filters.Sort "reads_asc"}}selected{{end}}>Reads asc</option>
          <option value="ratings_desc" {{if eq .Filters.Sort "ratings_desc"}}selected{{end}}>Ratings desc</option>
          <option value="ratings_asc" {{if eq .Filters.Sort "ratings_asc"}}selected{{end}}>Ratings asc</option>
        </select>
      </div>
      <div class="actions">
        <button type="submit">Apply</button>
        <a href="/admin/items">Reset</a>
      </div>
    </div>
  </form>
</section>
{{if .Items}}
<table>
  <thead><tr><th>Title</th><th>Feed</th><th>Time</th><th>Shows</th><th>Reads</th><th>Ratings</th><th>Avg Rating</th></tr></thead>
  <tbody>
    {{range .Items}}
    <tr>
      <td><a href="/admin/items/{{.ID}}">{{.Title}}</a></td>
      <td><a href="/admin/feeds/{{.FeedID}}">{{.FeedName}}</a></td>
      <td>{{formatTime .DisplayTime}}</td>
      <td>{{.ShowCount}}</td>
      <td>{{.ReadCount}}</td>
      <td>{{.RatingCount}}</td>
      <td>{{if gt .RatingCount 0}}{{formatFloat .AverageRating}}{{else}}-{{end}}</td>
    </tr>
    {{end}}
  </tbody>
</table>
{{template "pagination" .Pagination}}
{{else}}<p class="empty">No items available.</p>{{end}}
{{end}}
`

const itemDetailTemplate = `
{{define "item_detail"}}
<h1>{{.Item.Title}}</h1>
<div class="meta">
  <div class="card"><strong>Feed</strong><a href="/admin/feeds/{{.Item.FeedID}}">{{.Item.FeedName}}</a></div>
  <div class="card"><strong>Source URL</strong><a href="{{.Item.URL}}">{{.Item.URL}}</a></div>
  <div class="card"><strong>Time</strong>{{formatTime .Item.DisplayTime}}</div>
  <div class="card"><strong>Shows</strong>{{.Item.ShowCount}}</div>
  <div class="card"><strong>Reads</strong>{{.Item.ReadCount}}</div>
  <div class="card"><strong>Ratings</strong>{{.Item.RatingCount}}</div>
  <div class="card"><strong>Average Rating</strong>{{if gt .Item.RatingCount 0}}{{formatFloat .Item.AverageRating}}{{else}}-{{end}}</div>
</div>
<div class="grid">
  <section class="panel">
    <h2>Show Records</h2>
    {{if .Shows}}
    <table>
      <thead><tr><th>Time</th><th>Device</th></tr></thead>
      <tbody>
        {{range .Shows}}
        <tr>
          <td>{{formatTime .CreatedAt}}</td>
          <td><a href="/admin/devices/{{.DeviceID}}">{{.DeviceID}}</a></td>
        </tr>
        {{end}}
      </tbody>
    </table>
    {{else}}<p class="empty">No show records yet.</p>{{end}}
  </section>
  <section class="panel">
    <h2>Read Records</h2>
    {{if .Reads}}
    <table>
      <thead><tr><th>Time</th><th>Device</th></tr></thead>
      <tbody>
        {{range .Reads}}
        <tr>
          <td>{{formatTime .CreatedAt}}</td>
          <td><a href="/admin/devices/{{.DeviceID}}">{{.DeviceID}}</a></td>
        </tr>
        {{end}}
      </tbody>
    </table>
    {{else}}<p class="empty">No read records yet.</p>{{end}}
  </section>
  <section class="panel">
    <h2>Rating Records</h2>
    {{if .Ratings}}
    <table>
      <thead><tr><th>Time</th><th>Device</th><th>Rating</th></tr></thead>
      <tbody>
        {{range .Ratings}}
        <tr>
          <td>{{formatTime .CreatedAt}}</td>
          <td><a href="/admin/devices/{{.DeviceID}}">{{.DeviceID}}</a></td>
          <td>{{.Rating}}</td>
        </tr>
        {{end}}
      </tbody>
    </table>
    {{else}}<p class="empty">No rating records yet.</p>{{end}}
  </section>
</div>
{{end}}
`

const devicesListTemplate = `
{{define "devices_list"}}
<h1>Devices</h1>
{{if .Devices}}
<table>
  <thead><tr><th>Device</th><th>Last Seen</th><th>Current Item</th><th>Shows</th><th>Reads</th><th>Ratings</th><th>Last Show</th><th>Last Read</th><th>Last Rating</th></tr></thead>
  <tbody>
    {{range .Devices}}
    <tr>
      <td><a href="/admin/devices/{{.DeviceID}}">{{.DeviceID}}</a></td>
      <td>{{formatTime .LastSeen}}</td>
      <td>{{if .HasCurrentItem}}<a href="/admin/items/{{.CurrentItemValue}}">{{.CurrentItemTitle}}</a>{{else}}-{{end}}</td>
      <td>{{.ShowCount}}</td>
      <td>{{.ReadCount}}</td>
      <td>{{.RatingCount}}</td>
      <td>{{formatTime .LastShowAt}}</td>
      <td>{{formatTime .LastReadAt}}</td>
      <td>{{formatTime .LastRatingAt}}</td>
    </tr>
    {{end}}
  </tbody>
</table>
{{template "pagination" .Pagination}}
{{else}}<p class="empty">No devices registered.</p>{{end}}
{{end}}
`

const deviceDetailTemplate = `
{{define "device_detail"}}
<h1>{{.Device.DeviceID}}</h1>
<div class="meta">
  <div class="card"><strong>Last Seen</strong>{{formatTime .Device.LastSeen}}</div>
  <div class="card"><strong>Current Item</strong>{{if .Device.HasCurrentItem}}<a href="/admin/items/{{.Device.CurrentItemValue}}">{{.Device.CurrentItemTitle}}</a>{{else}}-{{end}}</div>
  <div class="card"><strong>Shows</strong>{{.Device.ShowCount}}</div>
  <div class="card"><strong>Reads</strong>{{.Device.ReadCount}}</div>
  <div class="card"><strong>Ratings</strong>{{.Device.RatingCount}}</div>
  <div class="card"><strong>Last Show</strong>{{formatTime .Device.LastShowAt}}</div>
  <div class="card"><strong>Last Read</strong>{{formatTime .Device.LastReadAt}}</div>
  <div class="card"><strong>Last Rating</strong>{{formatTime .Device.LastRatingAt}}</div>
</div>
<div class="grid">
  <section class="panel">
    <h2>Show Records</h2>
    {{if .Shows}}
    <table>
      <thead><tr><th>Time</th><th>Item</th><th>Feed</th></tr></thead>
      <tbody>
        {{range .Shows}}
        <tr>
          <td>{{formatTime .CreatedAt}}</td>
          <td><a href="/admin/items/{{.ItemID}}">{{.ItemTitle}}</a></td>
          <td>{{.FeedName}}</td>
        </tr>
        {{end}}
      </tbody>
    </table>
    {{else}}<p class="empty">No show records yet.</p>{{end}}
  </section>
  <section class="panel">
    <h2>Read Records</h2>
    {{if .Reads}}
    <table>
      <thead><tr><th>Time</th><th>Item</th><th>Feed</th></tr></thead>
      <tbody>
        {{range .Reads}}
        <tr>
          <td>{{formatTime .CreatedAt}}</td>
          <td><a href="/admin/items/{{.ItemID}}">{{.ItemTitle}}</a></td>
          <td>{{.FeedName}}</td>
        </tr>
        {{end}}
      </tbody>
    </table>
    {{else}}<p class="empty">No read records yet.</p>{{end}}
  </section>
  <section class="panel">
    <h2>Rating Records</h2>
    {{if .Ratings}}
    <table>
      <thead><tr><th>Time</th><th>Item</th><th>Rating</th></tr></thead>
      <tbody>
        {{range .Ratings}}
        <tr>
          <td>{{formatTime .CreatedAt}}</td>
          <td><a href="/admin/items/{{.ItemID}}">{{.ItemTitle}}</a></td>
          <td>{{.Rating}}</td>
        </tr>
        {{end}}
      </tbody>
    </table>
    {{else}}<p class="empty">No rating records yet.</p>{{end}}
  </section>
</div>
{{end}}
`
