package admin

import (
	"bytes"
	"errors"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/esp32-rss-display/backend/server/domain/admin"
)

const pageSize = 20

type layoutData struct {
	Title      string
	ActivePath string
	Content    template.HTML
}

type Handler struct {
	templates *template.Template
	readModel admin.ReadModelService
}

func NewHandler(readModel admin.ReadModelService) *Handler {
	return &Handler{
		templates: mustParseTemplates(),
		readModel: readModel,
	}
}

func Mount(mux *http.ServeMux, readModel admin.ReadModelService) {
	h := NewHandler(readModel)
	mux.HandleFunc("GET /admin", h.Dashboard)
	mux.HandleFunc("GET /admin/feeds", h.ListFeeds)
	mux.HandleFunc("GET /admin/feeds/{id}", h.ShowFeed)
	mux.HandleFunc("GET /admin/items", h.ListItems)
	mux.HandleFunc("GET /admin/items/{id}", h.ShowItem)
	mux.HandleFunc("GET /admin/devices", h.ListDevices)
	mux.HandleFunc("GET /admin/devices/{device_id}", h.ShowDevice)
}

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	view, err := h.readModel.Dashboard(r.Context())
	if err != nil {
		http.Error(w, "failed to load dashboard", http.StatusInternalServerError)
		return
	}

	h.render(w, "Dashboard", "/admin", "dashboard", view)
}

func (h *Handler) ListFeeds(w http.ResponseWriter, r *http.Request) {
	page := parsePage(r)
	view, err := h.readModel.ListFeeds(r.Context(), page, pageSize)
	if err != nil {
		http.Error(w, "failed to load feeds", http.StatusInternalServerError)
		return
	}

	// Build pagination URLs from request.
	view.Pagination = updatePaginationURLs(view.Pagination, r)
	h.render(w, "Feeds", "/admin/feeds", "feeds_list", view)
}

func (h *Handler) ShowFeed(w http.ResponseWriter, r *http.Request) {
	feedID, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	page := parsePage(r)
	view, err := h.readModel.FeedDetail(r.Context(), uint(feedID), page, pageSize)
	if errors.Is(err, admin.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "failed to load feed", http.StatusInternalServerError)
		return
	}

	view.Pagination = updatePaginationURLs(view.Pagination, r)
	h.render(w, "Feed Detail", "/admin/feeds", "feed_detail", view)
}

func (h *Handler) ListItems(w http.ResponseWriter, r *http.Request) {
	page := parsePage(r)
	filters, err := h.parseItemFilters(r)
	if err != nil {
		http.Error(w, "invalid item filters", http.StatusBadRequest)
		return
	}

	view, err := h.readModel.ListItems(r.Context(), filters, page, pageSize)
	if err != nil {
		http.Error(w, "failed to load items", http.StatusInternalServerError)
		return
	}

	view.Pagination = updatePaginationURLs(view.Pagination, r)
	h.render(w, "Items", "/admin/items", "items_list", view)
}

func (h *Handler) ShowItem(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	view, err := h.readModel.ItemDetail(r.Context(), uint(itemID))
	if errors.Is(err, admin.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "failed to load item", http.StatusInternalServerError)
		return
	}

	h.render(w, "Item Detail", "/admin/items", "item_detail", view)
}

func (h *Handler) ListDevices(w http.ResponseWriter, r *http.Request) {
	page := parsePage(r)
	view, err := h.readModel.ListDevices(r.Context(), page, pageSize)
	if err != nil {
		http.Error(w, "failed to load devices", http.StatusInternalServerError)
		return
	}

	view.Pagination = updatePaginationURLs(view.Pagination, r)
	h.render(w, "Devices", "/admin/devices", "devices_list", view)
}

func (h *Handler) ShowDevice(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		http.NotFound(w, r)
		return
	}

	view, err := h.readModel.DeviceDetail(r.Context(), deviceID)
	if errors.Is(err, admin.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "failed to load device", http.StatusInternalServerError)
		return
	}

	h.render(w, "Device Detail", "/admin/devices", "device_detail", view)
}

func (h *Handler) render(w http.ResponseWriter, title, activePath, bodyTemplate string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var body bytes.Buffer
	if err := h.templates.ExecuteTemplate(&body, bodyTemplate, data); err != nil {
		http.Error(w, "failed to render admin page", http.StatusInternalServerError)
		return
	}

	if err := h.templates.ExecuteTemplate(w, "layout", layoutData{
		Title:      title,
		ActivePath: activePath,
		Content:    template.HTML(body.String()),
	}); err != nil {
		http.Error(w, "failed to render admin page", http.StatusInternalServerError)
	}
}

func (h *Handler) parseItemFilters(r *http.Request) (admin.ItemFilters, error) {
	filters := admin.ItemFilters{Sort: admin.NormalizeItemSort(r.URL.Query().Get("sort"))}
	if title := strings.TrimSpace(r.URL.Query().Get("title")); title != "" {
		filters.Title = title
	}
	if rawFeedID := strings.TrimSpace(r.URL.Query().Get("feed_id")); rawFeedID != "" {
		feedID, err := strconv.ParseUint(rawFeedID, 10, 64)
		if err != nil {
			return admin.ItemFilters{}, err
		}
		filters.FeedID = uint(feedID)
	}
	for _, part := range []struct {
		raw    string
		target *string
	}{
		{raw: strings.TrimSpace(r.URL.Query().Get("from")), target: &filters.From},
		{raw: strings.TrimSpace(r.URL.Query().Get("to")), target: &filters.To},
	} {
		if part.raw == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", part.raw); err != nil {
			return admin.ItemFilters{}, err
		}
		*part.target = part.raw
	}

	feedOptions, err := h.readModel.FeedOptions(r.Context())
	if err != nil {
		return admin.ItemFilters{}, err
	}
	filters.FeedNames = feedOptions

	return filters, nil
}

func parsePage(r *http.Request) int {
	page, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("page")))
	if err != nil || page < 1 {
		return 1
	}
	return page
}

func updatePaginationURLs(p admin.Pagination, r *http.Request) admin.Pagination {
	if p.HasPrev {
		p.PrevURL = buildPageURL(r, p.Page-1)
	}
	if p.HasNext {
		p.NextURL = buildPageURL(r, p.Page+1)
	}
	return p
}

func buildPageURL(r *http.Request, page int) string {
	query := r.URL.Query()
	query.Set("page", strconv.Itoa(page))
	encoded := query.Encode()
	if encoded == "" {
		return r.URL.Path
	}
	return r.URL.Path + "?" + encoded
}