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
	"net/http"
	"time"

	"github.com/esp32-rss-display/backend/server/config"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

//go:embed fonts/wqy-microhei.ttf
var wqyMicroHeiTTF []byte

// colorPalette is a fixed set of dark background colors used when an RSS item
// has no image. All RGB components are <= 120 to ensure white text stays legible.
var colorPalette = []color.RGBA{
	{R: 31, G: 47, B: 84, A: 255},
	{R: 60, G: 32, B: 80, A: 255},
	{R: 15, G: 70, B: 60, A: 255},
	{R: 80, G: 30, B: 30, A: 255},
	{R: 55, G: 55, B: 20, A: 255},
	{R: 20, G: 50, B: 80, A: 255},
	{R: 70, G: 40, B: 15, A: 255},
	{R: 30, G: 70, B: 30, A: 255},
	{R: 80, G: 20, B: 60, A: 255},
	{R: 40, G: 40, B: 80, A: 255},
}

type Renderer struct {
	imageWidth    int
	imageHeight   int
	httpClient    *http.Client
	barFace       font.Face
	cardFace      font.Face
	cardTimeFace  font.Face
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

func NewRenderer(cfg *config.RSSConfig) *Renderer {
	timeout := time.Duration(cfg.ImageDownloadTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	return &Renderer{
		imageWidth:   cfg.ImageWidth,
		imageHeight:  cfg.ImageHeight,
		httpClient:   &http.Client{Timeout: timeout},
		barFace:      newWQYFace(13),
		cardFace:     newWQYFace(18),
		cardTimeFace: newWQYFace(11),
	}
}

// DownloadImage fetches the image at url and decodes it. Returns an error if
// the request fails, the status is not 200, or the format is unsupported.
func (r *Renderer) DownloadImage(url string) (image.Image, error) {
	resp, err := r.httpClient.Get(url)
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

// ResizeImage scales src to the configured output size using nearest-neighbour interpolation.
func (r *Renderer) ResizeImage(src image.Image) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, r.imageWidth, r.imageHeight))
	xdraw.NearestNeighbor.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)
	return dst
}

func (r *Renderer) EncodeJPEG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// GenerateColorCard creates a configured-size solid-color image whose colour is
// determined deterministically by the FNV-32a hash of title mod the palette.
func (r *Renderer) GenerateColorCard(title string) *image.RGBA {
	h := fnv.New32a()
	_, _ = h.Write([]byte(title))
	c := colorPalette[int(h.Sum32())%len(colorPalette)]

	img := image.NewRGBA(image.Rect(0, 0, r.imageWidth, r.imageHeight))
	for y := 0; y < r.imageHeight; y++ {
		for x := 0; x < r.imageWidth; x++ {
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

	fitRunes := func(runes []rune, suffix string) int {
		lo, hi := 0, len(runes)
		for lo < hi {
			mid := (lo + hi + 1) / 2
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
			n := fitRunes(runes, "...")
			lines = append(lines, string(runes[:n])+"...")
			break
		}

		breakAt := -1
		for i := len(runes) - 1; i > 0; i-- {
			if runes[i] == ' ' && fits(string(runes[:i])) {
				breakAt = i
				break
			}
		}
		if breakAt == -1 {
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

// OverlayText draws a semi-transparent black bar at the bottom of img
// (<= imgHeight/3) and renders the title (up to 3 wrapped lines) plus pubTime
// in white. Used when a real photo is present as the background.
func (r *Renderer) OverlayText(img *image.RGBA, title string, pubTime time.Time) {
	const (
		margin   = 8
		padTop   = 8
		barAlpha = 160
		maxRows  = 4
	)

	imgW := img.Bounds().Max.X
	imgH := img.Bounds().Max.Y
	maxBarHeight := imgH / 3
	lineHeight := (maxBarHeight - padTop) / maxRows

	face := r.barFace
	ascent := face.Metrics().Ascent.Round()
	maxTextWidth := imgW - 2*margin

	titleLines := wrapText(face, title, maxTextWidth, 3)
	timeStr := pubTime.UTC().Format("2006-01-02 15:04")

	totalRows := len(titleLines) + 1
	barHeight := totalRows*lineHeight + padTop
	if barHeight > maxBarHeight {
		barHeight = maxBarHeight
	}
	barY := imgH - barHeight

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

// OverlayTextFull renders the title and timestamp across the entire canvas.
// Used for color-card images (no background photo) so the full height is
// available for text. Title lines are centered horizontally and as a block
// vertically; the timestamp sits at the bottom in a smaller font.
func (r *Renderer) OverlayTextFull(img *image.RGBA, title string, pubTime time.Time) {
	const (
		margin        = 16
		bottomPad     = 12
		maxTitleLines = 5
	)

	imgW := img.Bounds().Max.X
	imgH := img.Bounds().Max.Y

	cardMetrics := r.cardFace.Metrics()
	timeMetrics := r.cardTimeFace.Metrics()
	timeSectionH := timeMetrics.Height.Round() + bottomPad + 8
	titleAreaH := imgH - timeSectionH
	cardLineH := cardMetrics.Height.Round() + 3

	maxWidth := imgW - 2*margin
	titleLines := wrapText(r.cardFace, title, maxWidth, maxTitleLines)

	totalTitleH := len(titleLines) * cardLineH
	titleBlockStartY := (titleAreaH - totalTitleH) / 2
	if titleBlockStartY < 8 {
		titleBlockStartY = 8
	}

	white := image.NewUniform(color.RGBA{R: 255, G: 255, B: 255, A: 255})
	d := &font.Drawer{Dst: img, Src: white}

	d.Face = r.cardFace
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

	timeStr := pubTime.UTC().Format("2006-01-02 15:04")
	d.Face = r.cardTimeFace
	timeW := d.MeasureString(timeStr).Round()
	timeX := (imgW - timeW) / 2
	if timeX < margin {
		timeX = margin
	}
	timeBaseline := imgH - bottomPad - timeMetrics.Descent.Round()
	d.Dot = fixed.Point26_6{X: fixed.I(timeX), Y: fixed.I(timeBaseline)}
	d.DrawString(timeStr)
}
