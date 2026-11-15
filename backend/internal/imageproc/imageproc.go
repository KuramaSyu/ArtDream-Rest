// Package imageproc renders derivative versions of uploaded artwork: format
// conversion (WebP by default), optional resizing and disk caching.
package imageproc

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/gen2brain/webp"
	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
)

// RenderOptions describes the requested derivative.
type RenderOptions struct {
	// Format is one of "webp" (default), "png", "jpg"/"jpeg".
	Format string
	// Width/Height are optional output bounds. Supplying only one preserves
	// the source aspect ratio.
	Width  int
	Height int
	// Quality is used for lossy formats. Zero selects a sensible default.
	Quality int
	// SourceMime lets the renderer skip re-encoding when the pristine upload
	// already matches the requested format.
	SourceMime string
}

// maxOutputPixels bounds the derivative size we are willing to allocate.
// It is deliberately below the upload limit: a full 120 MP source decoded at
// request time needs several hundred MB, and the rank/gallery pages request
// many variants at once.
const maxOutputPixels = 48_000_000

// Renderer owns the derivative cache directory.
type Renderer struct {
	CacheDir string

	// sem bounds how many decode/scale/encode pipelines run at the same time.
	// Without it a page load can start dozens of renders (each holding a full
	// decoded source and an encoder working buffer) and the process dies of
	// OOM.
	sem chan struct{}

	mu    sync.Mutex
	calls map[string]*renderCall
}

type renderCall struct {
	done        chan struct{}
	data        []byte
	contentType string
	err         error
}

// New creates a renderer, ensuring the cache directory exists.
func New(cacheDir string) (*Renderer, error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("create image cache: %w", err)
	}
	return &Renderer{
		CacheDir: cacheDir,
		sem:      make(chan struct{}, renderConcurrency()),
		calls:    make(map[string]*renderCall),
	}, nil
}

// renderConcurrency picks a conservative default: image decoding/scaling is
// memory bound, not CPU bound, so running more than a handful of pipelines in
// parallel only increases peak RSS. Override with IMAGE_RENDER_CONCURRENCY.
func renderConcurrency() int {
	if raw := strings.TrimSpace(os.Getenv("IMAGE_RENDER_CONCURRENCY")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return n
		}
	}
	n := runtime.GOMAXPROCS(0) / 2
	if n < 2 {
		n = 2
	}
	if n > 4 {
		n = 4
	}
	return n
}

// Render returns encoded image bytes and their content type. Results are
// cached by source identity plus render options.
func (r *Renderer) Render(sourcePath, imageID string, opts RenderOptions) ([]byte, string, error) {
	format := normalizeFormat(opts.Format)
	quality := clampQuality(format, opts.Quality)

	srcInfo, err := os.Stat(sourcePath)
	if err != nil {
		return nil, "", err
	}

	key := cacheKey(sourcePath, srcInfo, format, opts.Width, opts.Height, quality)
	dir := filepath.Join(r.CacheDir, safeID(imageID))
	outPath := filepath.Join(dir, key+"."+extension(format))
	if data, err := os.ReadFile(outPath); err == nil {
		return data, contentType(format), nil
	}

	// Fast path: the upload already is a WebP at native size and the caller
	// did not request a re-encode quality. Serving the pristine bytes keeps
	// the "original quality" guarantee literal.
	if format == "webp" && opts.Width == 0 && opts.Height == 0 && opts.Quality == 0 &&
		strings.EqualFold(opts.SourceMime, "image/webp") {
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			return nil, "", err
		}
		return data, contentType(format), nil
	}

	// Collapse duplicate requests (the same derivative is often requested
	// several times while a page loads) into a single decode/encode pass.
	call, leader := r.beginCall(key)
	if !leader {
		<-call.done
		return call.data, call.contentType, call.err
	}
	defer r.finishCall(key, call)

	data, ctype, err := r.renderGuarded(sourcePath, dir, outPath, format, quality, opts)
	call.data, call.contentType, call.err = data, ctype, err
	return data, ctype, err
}

// renderGuarded turns a panic in the image pipeline into an error so that
// goroutines waiting on the same derivative don't receive an empty result.
func (r *Renderer) renderGuarded(sourcePath, dir, outPath, format string, quality int, opts RenderOptions) (data []byte, ctype string, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			data, ctype, err = nil, "", fmt.Errorf("render panic: %v", rec)
		}
	}()
	return r.render(sourcePath, dir, outPath, format, quality, opts)
}

// beginCall registers an in-flight render for key. The first caller becomes
// the leader and performs the work; everyone else waits for its result.
func (r *Renderer) beginCall(key string) (*renderCall, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if call, ok := r.calls[key]; ok {
		return call, false
	}
	call := &renderCall{done: make(chan struct{})}
	r.calls[key] = call
	return call, true
}

func (r *Renderer) finishCall(key string, call *renderCall) {
	r.mu.Lock()
	delete(r.calls, key)
	r.mu.Unlock()
	close(call.done)
}

func (r *Renderer) render(sourcePath, dir, outPath, format string, quality int, opts RenderOptions) ([]byte, string, error) {
	// Bound concurrent pipelines before allocating any image buffers.
	r.sem <- struct{}{}
	defer func() { <-r.sem }()

	src, err := decodeFile(sourcePath)
	if err != nil {
		return nil, "", fmt.Errorf("decode source image: %w", err)
	}

	srcW, srcH := src.Bounds().Dx(), src.Bounds().Dy()
	dstW, dstH := targetSize(srcW, srcH, opts.Width, opts.Height)
	if dstW <= 0 || dstH <= 0 {
		return nil, "", fmt.Errorf("invalid target size %dx%d", dstW, dstH)
	}
	if int64(dstW)*int64(dstH) > maxOutputPixels {
		return nil, "", fmt.Errorf("target size %dx%d exceeds the render budget", dstW, dstH)
	}

	img := src
	if dstW != srcW || dstH != srcH {
		dst := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
		img = dst
	}

	var buf bytes.Buffer
	switch format {
	case "png":
		enc := png.Encoder{CompressionLevel: png.BestSpeed}
		if err := enc.Encode(&buf, img); err != nil {
			return nil, "", err
		}
	case "jpg":
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
			return nil, "", err
		}
	default:
		if err := webp.Encode(&buf, img, webp.Options{Quality: quality}); err != nil {
			return nil, "", err
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, "", err
	}
	if err := writeAtomic(outPath, buf.Bytes()); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), contentType(format), nil
}

// RemoveForImage deletes all cached derivatives of an image.
func (r *Renderer) RemoveForImage(imageID string) error {
	return os.RemoveAll(filepath.Join(r.CacheDir, safeID(imageID)))
}

// DecodeConfig validates an upload and reports its format and dimensions.
func DecodeConfig(data []byte) (image.Config, string, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	return cfg, format, err
}

func decodeFile(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(io.Reader(f))
	return img, err
}

func normalizeFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "webp":
		return "webp"
	case "png":
		return "png"
	case "jpg", "jpeg":
		return "jpg"
	default:
		return "webp"
	}
}

func extension(format string) string {
	if format == "jpg" {
		return "jpg"
	}
	return format
}

func contentType(format string) string {
	switch format {
	case "png":
		return "image/png"
	case "jpg":
		return "image/jpeg"
	default:
		return "image/webp"
	}
}

func clampQuality(format string, q int) int {
	if format == "png" {
		return 0
	}
	if q <= 0 {
		if format == "jpg" {
			return 88
		}
		return 92
	}
	if q > 100 {
		return 100
	}
	if q < 1 {
		return 1
	}
	return q
}

func targetSize(srcW, srcH, reqW, reqH int) (int, int) {
	switch {
	case reqW > 0 && reqH > 0:
		// Explicit two-axis requests are honoured, but never above the source
		// dimensions: `?w=8192&h=8192` used to allocate a multi-hundred-MB
		// upscaled NRGBA buffer for a small original.
		if reqW > srcW {
			reqW = srcW
		}
		if reqH > srcH {
			reqH = srcH
		}
		return reqW, reqH
	case reqW > 0:
		if reqW > srcW {
			reqW = srcW
		}
		return reqW, maxInt(1, int(float64(srcH)*float64(reqW)/float64(srcW)+0.5))
	case reqH > 0:
		if reqH > srcH {
			reqH = srcH
		}
		return maxInt(1, int(float64(srcW)*float64(reqH)/float64(srcH)+0.5)), reqH
	default:
		return srcW, srcH
	}
}

func cacheKey(sourcePath string, info os.FileInfo, format string, w, h, q int) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("%s|%d|%d|%s|%d|%d|%d",
		sourcePath, info.Size(), info.ModTime().UnixNano(), format, w, h, q)))
	return hex.EncodeToString(sum[:])
}

func safeID(id string) string {
	var b strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
