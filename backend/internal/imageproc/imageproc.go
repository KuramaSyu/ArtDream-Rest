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
	"strings"

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

// Renderer owns the derivative cache directory.
type Renderer struct {
	CacheDir string
}

// New creates a renderer, ensuring the cache directory exists.
func New(cacheDir string) (*Renderer, error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("create image cache: %w", err)
	}
	return &Renderer{CacheDir: cacheDir}, nil
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

	src, err := decodeFile(sourcePath)
	if err != nil {
		return nil, "", fmt.Errorf("decode source image: %w", err)
	}

	srcW, srcH := src.Bounds().Dx(), src.Bounds().Dy()
	dstW, dstH := targetSize(srcW, srcH, opts.Width, opts.Height)
	if dstW <= 0 || dstH <= 0 {
		return nil, "", fmt.Errorf("invalid target size %dx%d", dstW, dstH)
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
		// Explicit two-axis requests are honoured verbatim (clients may want
		// a specific crop box), but clamp the single-axis paths below so the
		// UI never decodes/encodes an upscaled copy by accident.
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
