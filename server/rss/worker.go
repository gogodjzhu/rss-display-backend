package rssworker

import (
	"bytes"
	_ "embed"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/esp32-rss-display/backend/server/config"
	"github.com/esp32-rss-display/backend/server/database"
	"github.com/esp32-rss-display/backend/server/metrics"
	"github.com/esp32-rss-display/backend/server/models"
	"github.com/mmcdole/gofeed"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/net/html"
)

//go:embed fonts/wqy-microhei.ttf
var wqyMicroHeiTTF []byte

// colorPalette is a fixed set of dark background colors used when an RSS item
// has no image. All RGB components are ≤ 120 to ensure white text stays legible.
var colorPalette = []color.RGBA{
	{R: 31, G: 47, B: 84, A: 255},  // dark navy
	{R: 60, G: 32, B: 80, A: 255},  // dark purple
	{R: 15, G: 70, B: 60, A: 255},  // dark teal
	{R: 80, G: 30, B: 30, A: 255},  // dark crimson
	{R: 55, G: 55, B: 20, A: 255},  // dark olive
	{R: 20, G: 50, B: 80, A: 255},  // dark steel
	{R: 70, G: 40, B: 15, A: 255},  // dark amber
	{R: 30, G: 70, B: 30, A: 255},  // dark forest
	{R: 80, G: 20, B: 60, A: 255},  // dark magenta
	{R: 40, G: 40, B: 80, A: 255},  // dark indigo
}

type Worker struct {
	fetchInterval time.Duration
	imageWidth    int
	imageHeight   int
	imageDir      string
	stopCh        chan struct{}
	// barFace is used for the 33%-height overlay bar on real images (13 pt).
	barFace font.Face
	// cardFace is used for the title on full-canvas color-card images (18 pt).
	cardFace font.Face
	// cardTimeFace is used for the timestamp on full-canvas color-card images (11 pt).
	cardTimeFace font.Face
}

// newWQYFace parses the embedded WenQuanYi Micro Hei TTF and returns a
// font.Face at sizePt points (72 DPI). Supports CJK + Latin.
func newWQYFace(sizePt float64) font.Face {
	tt, err := opentype.Parse(wqyMicroHeiTTF)
	if err != nil {
		panic("rssworker: failed to parse embedded WQY font: " + err.Error())
	}
	face, err := opentype.NewFace(tt, &opentype.FaceOptions{Size: sizePt, DPI: 72})
	if err != nil {
		panic("rssworker: failed to create font face: " + err.Error())
	}
	return face
}

func New(cfg *config.RSSConfig, imageDir string) *Worker {
	return &Worker{
		fetchInterval: time.Duration(cfg.FetchIntervalMinutes) * time.Minute,
		imageWidth:    cfg.ImageWidth,
		imageHeight:   cfg.ImageHeight,
		imageDir:      imageDir,
		stopCh:        make(chan struct{}),
		// 13 pt: 4 rows (3 title + 1 time) fill imgHeight/3 comfortably.
		barFace: newWQYFace(13),
		// 18 pt: large, readable title for full-canvas color cards.
		cardFace: newWQYFace(18),
		// 11 pt: compact timestamp at the bottom of color cards.
		cardTimeFace: newWQYFace(11),
	}
}

func (w *Worker) Start() {
	w.backfillImages()
	w.fetchAllFeeds()
	go w.loop()
}

func (w *Worker) Stop() {
	close(w.stopCh)
}

func (w *Worker) loop() {
	ticker := time.NewTicker(w.fetchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.fetchAllFeeds()
		case <-w.stopCh:
			return
		}
	}
}

// backfillImages repairs items that are missing a usable local image file:
//
//   - Items whose image_path is a remote URL (created by an older code version):
//     re-download and save locally without text overlay.
//   - Items whose image_path is empty or points to a non-existent file
//     (created when no image was found, or file was deleted):
//     generate a color card with title+time overlay and save locally.
func (w *Worker) backfillImages() {
	db := database.GetDB()

	// --- Pass 1: remote URLs → download and save locally (no overlay) ---
	var remoteItems []models.Item
	if err := db.Where("image_path LIKE 'http://%' OR image_path LIKE 'https://%'").Find(&remoteItems).Error; err != nil {
		log.Printf("[backfill] error querying remote-URL items: %v", err)
	} else if len(remoteItems) > 0 {
		log.Printf("[backfill] re-downloading images for %d items", len(remoteItems))
		for _, item := range remoteItems {
			pubDate := item.CreatedAt
			if item.PublishedAt != nil {
				pubDate = *item.PublishedAt
			}
			src, err := w.downloadImage(item.ImagePath)
			if err != nil {
				log.Printf("[backfill] failed to download image for item %d: %s", item.ID, item.ImagePath)
				continue
			}
			resized := w.resizeImage(src, w.imageWidth, w.imageHeight)
			localPath := w.saveImage(resized, w.imageDir, pubDate)
			if localPath == "" {
				log.Printf("[backfill] failed to save image for item %d", item.ID)
				continue
			}
			if err := db.Model(&item).Update("image_path", localPath).Error; err != nil {
				log.Printf("[backfill] failed to update item %d: %v", item.ID, err)
			} else {
				log.Printf("[backfill] item %d image saved to %s", item.ID, localPath)
			}
		}
	}

	// --- Pass 2: empty or missing local paths → color card + overlay ---
	var allItems []models.Item
	if err := db.Where("image_path = '' OR image_path IS NULL").Find(&allItems).Error; err != nil {
		log.Printf("[backfill] error querying empty-path items: %v", err)
		return
	}
	// Also check items whose image_path points to a file that no longer exists.
	var localItems []models.Item
	if err := db.Where("image_path != '' AND image_path NOT LIKE 'http://%' AND image_path NOT LIKE 'https://%'").Find(&localItems).Error; err != nil {
		log.Printf("[backfill] error querying local-path items: %v", err)
	} else {
		for _, item := range localItems {
			if _, err := os.Stat(item.ImagePath); os.IsNotExist(err) {
				allItems = append(allItems, item)
			}
		}
	}

	if len(allItems) == 0 {
		return
	}
	log.Printf("[backfill] generating color-card images for %d items", len(allItems))
	for _, item := range allItems {
		pubDate := item.CreatedAt
		if item.PublishedAt != nil {
			pubDate = *item.PublishedAt
		}
		canvas := w.generateColorCard(item.Title, w.imageWidth, w.imageHeight)
		w.overlayTextFull(canvas, item.Title, pubDate)
		localPath := w.saveImage(canvas, w.imageDir, pubDate)
		if localPath == "" {
			log.Printf("[backfill] failed to save color card for item %d", item.ID)
			continue
		}
		if err := db.Model(&item).Update("image_path", localPath).Error; err != nil {
			log.Printf("[backfill] failed to update item %d: %v", item.ID, err)
		} else {
			log.Printf("[backfill] item %d color card saved to %s", item.ID, localPath)
		}
	}
}

func (w *Worker) fetchAllFeeds() {
	db := database.GetDB()
	var feeds []models.Feed

	if err := db.Where("enabled = ?", true).Find(&feeds).Error; err != nil {
		log.Printf("Error fetching feeds: %v", err)
		metrics.RSSFetchError.Add(1)
		return
	}

	for _, feed := range feeds {
		w.fetchFeed(feed)
	}
}

func (w *Worker) fetchFeed(feed models.Feed) {
	metrics.RSSFetchTotal.Add(1)

	parser := gofeed.NewParser()
	parsed, err := parser.ParseURL(feed.URL)
	if err != nil {
		log.Printf("Error parsing feed %s: %v", feed.Name, err)
		metrics.RSSFetchError.Add(1)
		return
	}

	db := database.GetDB()

	for _, item := range parsed.Items {
		title := item.Title
		url := item.Link

		if url == "" {
			continue
		}

		var existingItem models.Item
		if err := db.Where("feed_id = ? AND url = ?", feed.ID, url).First(&existingItem).Error; err == nil {
			continue
		}

		var publishedAt *time.Time
		if item.PublishedParsed != nil {
			t := *item.PublishedParsed
			publishedAt = &t
		}

		pubDate := time.Now()
		if publishedAt != nil {
			pubDate = *publishedAt
		}

		// Build the processed image: either downloaded+resized or a color card.
		// The overlay style depends on whether a real photo was obtained.
		var canvas *image.RGBA
		hasImage := false

		imageURL := w.extractImage(item)
		if imageURL != "" {
			src, err := w.downloadImage(imageURL)
			if err == nil {
				canvas = w.resizeImage(src, w.imageWidth, w.imageHeight)
				hasImage = true
			} else {
				log.Printf("[image] download failed for %s, falling back to color card: %v", url, err)
			}
		}

		if !hasImage {
			canvas = w.generateColorCard(title, w.imageWidth, w.imageHeight)
			w.overlayTextFull(canvas, title, pubDate)
		} else {
			w.overlayText(canvas, title, pubDate)
		}
		imagePath := w.saveImage(canvas, w.imageDir, pubDate)

		newItem := models.Item{
			FeedID:      feed.ID,
			Title:       title,
			URL:         url,
			ImagePath:   imagePath,
			PublishedAt: publishedAt,
		}

		if err := db.Create(&newItem).Error; err != nil {
			log.Printf("Error saving item: %v", err)
			continue
		}

		metrics.RSSItemsParsedTotal.Add(1)
	}
}

// downloadImage fetches the image at url and decodes it. Returns an error if
// the request fails, the status is not 200, or the format is unsupported.
func (w *Worker) downloadImage(url string) (image.Image, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	img, format, err := image.Decode(resp.Body)
	if err != nil {
		return nil, err
	}
	if format != "jpeg" && format != "png" {
		return nil, fmt.Errorf("unsupported image format: %s", format)
	}
	return img, nil
}

// resizeImage scales src to width×height using nearest-neighbour interpolation.
func (w *Worker) resizeImage(src image.Image, width, height int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	xdraw.NearestNeighbor.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)
	return dst
}

// saveImage encodes img as JPEG (quality 80) and writes it under
// <dir>/YYYYMMDD/<nanosecond>.jpg. Returns the full path on success or "".
func (w *Worker) saveImage(img *image.RGBA, dir string, date time.Time) string {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		return ""
	}

	dateDir := filepath.Join(dir, date.UTC().Format("20060102"))
	if err := os.MkdirAll(dateDir, 0755); err != nil {
		return ""
	}

	filename := filepath.Join(dateDir, fmt.Sprintf("%d.jpg", time.Now().UnixNano()))
	if err := os.WriteFile(filename, buf.Bytes(), 0644); err != nil {
		return ""
	}
	return filename
}

// generateColorCard creates a width×height solid-color image whose colour is
// determined deterministically by the FNV-32a hash of title mod the palette.
func (w *Worker) generateColorCard(title string, width, height int) *image.RGBA {
	h := fnv.New32a()
	h.Write([]byte(title))
	c := colorPalette[int(h.Sum32())%len(colorPalette)]

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

// wrapText splits text into at most maxLines lines that each fit within
// maxWidth pixels as measured by face. It prefers to break at spaces; if no
// suitable space is found it hard-breaks using binary search. The last allowed
// line is truncated with "..." if the text still overflows.
func wrapText(face font.Face, text string, maxWidth, maxLines int) []string {

	d := &font.Drawer{Face: face}
	fits := func(s string) bool { return d.MeasureString(s).Round() <= maxWidth }

	// fitRunes returns the largest prefix of runes whose rendering (plus
	// optional suffix) fits within maxWidth.
	fitRunes := func(runes []rune, suffix string) int {
		lo, hi := 0, len(runes)
		for lo < hi {
			mid := (lo+hi+1)/2
			if fits(string(runes[:mid]) + suffix) {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		return lo
	}

	runes := []rune(text)
	var lines []string

	for len(runes) > 0 {
		full := string(runes)
		if fits(full) {
			lines = append(lines, full)
			break
		}

		if len(lines) == maxLines-1 {
			// Last line: truncate to fit with "...".
			n := fitRunes(runes, "...")
			lines = append(lines, string(runes[:n])+"...")
			break
		}

		// Find the last space at which the prefix fits.
		breakAt := -1
		for i := len(runes) - 1; i > 0; i-- {
			if runes[i] == ' ' && fits(string(runes[:i])) {
				breakAt = i
				break
			}
		}
		if breakAt == -1 {
			// No usable space — hard-break at the character boundary.
			breakAt = fitRunes(runes, "")
			if breakAt == 0 {
				breakAt = 1
			}
		}

		lines = append(lines, string(runes[:breakAt]))
		runes = runes[breakAt:]
		for len(runes) > 0 && runes[0] == ' ' {
			runes = runes[1:]
		}
	}

	return lines
}

// overlayText draws a semi-transparent black bar at the bottom of img
// (≤ imgHeight/3) and renders the title (up to 3 wrapped lines) plus pubTime
// in white. Used when a real photo is present as the background.
func (w *Worker) overlayText(img *image.RGBA, title string, pubTime time.Time) {
	const (
		margin   = 8   // left/right pixel margin for text
		padTop   = 8   // padding above the first text line inside the bar
		barAlpha = 160 // semi-transparent black overlay (≈63%)
		maxRows  = 4   // max title rows (3) + time row (1)
	)

	imgW := img.Bounds().Max.X
	imgH := img.Bounds().Max.Y
	maxBarHeight := imgH / 3

	// Compute a line height that makes maxRows fill exactly maxBarHeight.
	lineHeight := (maxBarHeight - padTop) / maxRows

	face := w.barFace
	ascent := face.Metrics().Ascent.Round()
	maxTextWidth := imgW - 2*margin

	titleLines := wrapText(face, title, maxTextWidth, 3)
	timeStr := pubTime.UTC().Format("2006-01-02 15:04")

	totalRows := len(titleLines) + 1 // title lines + 1 time row
	barHeight := totalRows*lineHeight + padTop
	if barHeight > maxBarHeight {
		barHeight = maxBarHeight
	}
	barY := imgH - barHeight

	// Blend a semi-transparent black rectangle over the bar region.
	for y := barY; y < imgH; y++ {
		for x := 0; x < imgW; x++ {
			orig := img.RGBAAt(x, y)
			img.SetRGBA(x, y, color.RGBA{
				R: uint8((int(orig.R) * (255 - barAlpha)) / 255),
				G: uint8((int(orig.G) * (255 - barAlpha)) / 255),
				B: uint8((int(orig.B) * (255 - barAlpha)) / 255),
				A: 255,
			})
		}
	}

	white := image.NewUniform(color.RGBA{R: 255, G: 255, B: 255, A: 255})

	drawRow := func(row int, text string) {
		baseline := barY + padTop + row*lineHeight + ascent
		d := &font.Drawer{
			Dst:  img,
			Src:  white,
			Face: face,
			Dot:  fixed.Point26_6{X: fixed.I(margin), Y: fixed.I(baseline)},
		}
		d.DrawString(text)
	}

	for i, line := range titleLines {
		drawRow(i, line)
	}
	drawRow(len(titleLines), timeStr)
}

// overlayTextFull renders the title and timestamp across the entire canvas.
// Used for color-card images (no background photo) so the full height is
// available for text. Title lines are centered horizontally and as a block
// vertically; the timestamp sits at the bottom in a smaller font.
func (w *Worker) overlayTextFull(img *image.RGBA, title string, pubTime time.Time) {
	const (
		margin      = 16  // left/right margin for text
		bottomPad   = 12  // pixels below the timestamp baseline
		maxTitleLines = 5 // allow more lines since the whole canvas is free
	)

	imgW := img.Bounds().Max.X
	imgH := img.Bounds().Max.Y

	cardMetrics := w.cardFace.Metrics()
	timeMetrics := w.cardTimeFace.Metrics()

	// Reserve space at the bottom for the timestamp.
	timeSectionH := timeMetrics.Height.Round() + bottomPad + 8 // 8 px gap above
	titleAreaH := imgH - timeSectionH

	// Line height for title: use the face's own height metric plus a small gap.
	cardLineH := cardMetrics.Height.Round() + 3

	maxWidth := imgW - 2*margin
	titleLines := wrapText(w.cardFace, title, maxWidth, maxTitleLines)

	// Vertically center the title block inside titleAreaH.
	totalTitleH := len(titleLines) * cardLineH
	titleBlockStartY := (titleAreaH - totalTitleH) / 2
	if titleBlockStartY < 8 {
		titleBlockStartY = 8
	}

	white := image.NewUniform(color.RGBA{R: 255, G: 255, B: 255, A: 255})
	d := &font.Drawer{Dst: img, Src: white}

	// Draw each title line centered horizontally.
	d.Face = w.cardFace
	for i, line := range titleLines {
		baseline := titleBlockStartY + i*cardLineH + cardMetrics.Ascent.Round()
		lineW := d.MeasureString(line).Round()
		startX := (imgW - lineW) / 2
		if startX < margin {
			startX = margin
		}
		d.Dot = fixed.Point26_6{X: fixed.I(startX), Y: fixed.I(baseline)}
		d.DrawString(line)
	}

	// Draw the timestamp centered at the bottom.
	timeStr := pubTime.UTC().Format("2006-01-02 15:04")
	d.Face = w.cardTimeFace
	timeW := d.MeasureString(timeStr).Round()
	timeX := (imgW - timeW) / 2
	if timeX < margin {
		timeX = margin
	}
	// Baseline: imgH - bottomPad - descent
	timeBaseline := imgH - bottomPad - timeMetrics.Descent.Round()
	d.Dot = fixed.Point26_6{X: fixed.I(timeX), Y: fixed.I(timeBaseline)}
	d.DrawString(timeStr)
}

func (w *Worker) extractImage(item *gofeed.Item) string {
	if item == nil {
		return ""
	}

	// Stage 1: <media:thumbnail> — used by BBC, Yahoo, and many major news feeds
	if imgURL := w.extractFromMediaThumbnail(item); imgURL != "" {
		log.Printf("[image] media:thumbnail: %s", imgURL)
		return imgURL
	}

	// Stage 2: <media:content> with medium="image" — another Media RSS variant
	if imgURL := w.extractFromMediaContent(item); imgURL != "" {
		log.Printf("[image] media:content: %s", imgURL)
		return imgURL
	}

	// Stage 3: gofeed native item.Image field
	if imgURL := w.extractFromItemImage(item); imgURL != "" {
		log.Printf("[image] item.Image: %s", imgURL)
		return imgURL
	}

	// Stage 4: RSS <enclosure type="image/...">
	if imgURL := w.extractFromEnclosures(item); imgURL != "" {
		log.Printf("[image] enclosure: %s", imgURL)
		return imgURL
	}

	// Stage 5: <img src> inside item HTML content / description
	content := item.Content
	if content == "" {
		content = item.Description
	}
	if imgURL := w.extractImgFromHTML(content); imgURL != "" {
		log.Printf("[image] html img: %s", imgURL)
		return imgURL
	}

	// Stage 6: iTunes-style <itunes:image href="...">
	if imgURL := w.extractFromITunes(item); imgURL != "" {
		log.Printf("[image] itunes:image: %s", imgURL)
		return imgURL
	}

	// Stage 7: scrape the linked article page for og:image / twitter:image
	if item.Link != "" {
		if imgURL := w.extractFromWebPage(item.Link); imgURL != "" {
			log.Printf("[image] webpage: %s", imgURL)
			return imgURL
		}
	}

	log.Printf("[image] no image found for item: %s", item.Link)
	return ""
}

// extractFromMediaThumbnail handles <media:thumbnail url="..."> used by BBC,
// Yahoo Media RSS, Reuters, and many other major news publishers.
func (w *Worker) extractFromMediaThumbnail(item *gofeed.Item) string {
	if item.Extensions == nil {
		return ""
	}
	mediaNS, ok := item.Extensions["media"]
	if !ok {
		return ""
	}
	thumbnails, ok := mediaNS["thumbnail"]
	if !ok || len(thumbnails) == 0 {
		return ""
	}
	// Pick the largest thumbnail if multiple are present
	best := ""
	bestArea := 0
	for _, t := range thumbnails {
		u := t.Attrs["url"]
		if !w.isValidImageURL(u) {
			continue
		}
		tw, th := parseIntAttr(t.Attrs["width"]), parseIntAttr(t.Attrs["height"])
		area := tw * th
		if area > bestArea || best == "" {
			best = u
			bestArea = area
		}
	}
	return best
}

// extractFromMediaContent handles <media:content url="..." medium="image">.
func (w *Worker) extractFromMediaContent(item *gofeed.Item) string {
	if item.Extensions == nil {
		return ""
	}
	mediaNS, ok := item.Extensions["media"]
	if !ok {
		return ""
	}
	contents, ok := mediaNS["content"]
	if !ok || len(contents) == 0 {
		return ""
	}
	best := ""
	bestArea := 0
	for _, c := range contents {
		medium := c.Attrs["medium"]
		typ := c.Attrs["type"]
		// Accept if medium="image" or type starts with "image/"
		if medium != "image" && !strings.HasPrefix(typ, "image/") {
			continue
		}
		u := c.Attrs["url"]
		if !w.isValidImageURL(u) {
			continue
		}
		pw, ph := parseIntAttr(c.Attrs["width"]), parseIntAttr(c.Attrs["height"])
		area := pw * ph
		if area > bestArea || best == "" {
			best = u
			bestArea = area
		}
	}
	return best
}

// extractFromItemImage uses gofeed's native item.Image field, which some
// parsers populate from <image> child elements inside an <item>.
func (w *Worker) extractFromItemImage(item *gofeed.Item) string {
	if item.Image != nil && w.isValidImageURL(item.Image.URL) {
		return item.Image.URL
	}
	return ""
}

// parseIntAttr converts a string attribute value to int, returning 0 on error.
func parseIntAttr(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func (w *Worker) extractFromEnclosures(item *gofeed.Item) string {
	if item.Enclosures == nil || len(item.Enclosures) == 0 {
		return ""
	}

	for _, enc := range item.Enclosures {
		if strings.HasPrefix(enc.Type, "image/") && enc.URL != "" {
			return enc.URL
		}
	}

	return ""
}

func (w *Worker) extractFromITunes(item *gofeed.Item) string {
	content := item.Content
	if content == "" {
		content = item.Description
	}

	if content == "" {
		return ""
	}

	itunesImgPattern := regexp.MustCompile(`<itunes:image[^>]+href=["']([^"']+)["']`)
	if matches := itunesImgPattern.FindStringSubmatch(content); len(matches) > 1 {
		return matches[1]
	}

	return ""
}

func (w *Worker) extractImgFromHTML(htmlContent string) string {
	if htmlContent == "" {
		return ""
	}

	reader := strings.NewReader(htmlContent)
	tokenizer := html.NewTokenizer(reader)

	for {
		token := tokenizer.Next()
		if token == html.ErrorToken {
			break
		}

		if token == html.StartTagToken || token == html.SelfClosingTagToken {
			t := tokenizer.Token()
			if t.Data == "img" {
				for _, attr := range t.Attr {
					if attr.Key == "src" {
						url := attr.Val
						if w.isValidImageURL(url) {
							return url
						}
					}
					if attr.Key == "data-src" || attr.Key == "data-original" || attr.Key == "data-lazy-src" {
						url := attr.Val
						if w.isValidImageURL(url) {
							return url
						}
					}
				}
			}
		}
	}

	return ""
}

func (w *Worker) isValidImageURL(rawURL string) bool {
	if rawURL == "" {
		return false
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return false
	}
	// Check only the path component (before any '?') to avoid false positives
	// on query parameters like "?format=webp".
	path := rawURL
	if idx := strings.IndexByte(rawURL, '?'); idx != -1 {
		path = rawURL[:idx]
	}
	lower := strings.ToLower(path)
	// Reject formats that Go's image decoder cannot handle, or that are
	// generally unsuitable for display (animated gif, vector svg).
	if strings.HasSuffix(lower, ".gif") || strings.HasSuffix(lower, ".svg") {
		return false
	}
	return true
}

var (
	ogImagePattern   = regexp.MustCompile(`(?i)<meta[^>]+property=["']og:image["'][^>]+content=["']([^"']+)["']`)
	ogImagePattern2  = regexp.MustCompile(`(?i)<meta[^>]+content=["']([^"']+)["'][^>]+property=["']og:image["']`)
	twitterImagePat  = regexp.MustCompile(`(?i)<meta[^>]+name=["']twitter:image["'][^>]+content=["']([^"']+)["']`)
	twitterImagePat2 = regexp.MustCompile(`(?i)<meta[^>]+content=["']([^"']+)["'][^>]+name=["']twitter:image["']`)
	itemPropPattern  = regexp.MustCompile(`(?i)<meta[^>]+itemprop=["']image["'][^>]+content=["']([^"']+)["']`)
	itemPropPattern2 = regexp.MustCompile(`(?i)<meta[^>]+content=["']([^"']+)["'][^>]+itemprop=["']image["']`)
)

func (w *Worker) extractFromWebPage(url string) string {
	if url == "" {
		return ""
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; RSS-Bot/1.0)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 50000))
	if err != nil {
		return ""
	}

	htmlContent := string(body)

	patterns := []*regexp.Regexp{ogImagePattern, ogImagePattern2, twitterImagePat, twitterImagePat2, itemPropPattern, itemPropPattern2}
	for _, pattern := range patterns {
		if matches := pattern.FindStringSubmatch(htmlContent); len(matches) > 1 {
			url := matches[1]
			if w.isValidImageURL(url) {
				return url
			}
		}
	}

	if imgURL := w.extractImgFromHTML(htmlContent); imgURL != "" {
		return imgURL
	}

	return ""
}
