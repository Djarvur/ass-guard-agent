package runtime

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strconv"

	"golang.org/x/image/draw"

	// Side-effect registration: x/image's webp decoder joins image.Decode/
	// image.DecodeConfig (D-09's accepted media set; decode-only — over-limit
	// webp re-encodes as JPEG below).
	_ "golang.org/x/image/webp"
)

// ImageLimits are the provider image constraints the ingress validates
// against (21-05, PAR-06). The defaults pin D-09's discretion to Anthropic's
// documented classes: 8000×8000 max dimensions, 10 MB per-image byte cap, and
// the 1568 px long-edge downscale target (the documented recommendation) with
// JPEG re-encode at quality 85 when re-encoding is required.
type ImageLimits struct {
	MaxW           int
	MaxH           int
	MaxBytes       int64
	TargetLongEdge int
}

// DefaultImageLimits is the pinned default limit set (Anthropic's documented
// vision classes — the only provider shape exercised by PAR-06).
//
//nolint:gochecknoglobals,mnd // a read-only value table; the numbers are D-09's pinned documented values
var DefaultImageLimits = ImageLimits{
	MaxW:           8000,
	MaxH:           8000,
	MaxBytes:       10 << 20,
	TargetLongEdge: 1568,
}

// maxDecodePixels is the decode-safety ceiling (Pitfall 6): an image whose
// declared pixel count exceeds it is refused BEFORE any pixel allocation —
// downscaling it would require decoding a frame the process cannot hold
// (a 40000×40000 bomb declares 1.6 Gpx ≈ 6.4 GB RGBA). Over the PROVIDER
// limits but within this ceiling → downscale; over this ceiling → loud refuse.
const maxDecodePixels = 100_000_000

// jpegRescaleQuality is D-09's pinned re-encode quality.
const jpegRescaleQuality = 85

// imgJPEGMedia is the canonical JPEG media type (the re-encode fallback's
// output tag).
const imgJPEGMedia = "image/jpeg"

// Ingress-failure stages: where ValidateAndScaleImage refused. The config
// stage is the pixel-bomb proof — a config-stage refusal NEVER decodes pixels.
const (
	stageConfig = "config"
	stageDecode = "decode"
	stageEncode = "encode"
)

// Ingress-failure classes (the loud-note vocabulary — outcome classes, never
// byte content).
const (
	imgErrUndecodable  = "undecodable image data"
	imgErrDecodeUnsafe = "dimensions exceed the decode-safety ceiling"
	imgErrUnshrinkable = "byte size cannot fit the cap after downscale"
)

// ImageError is the typed ingress-failure class (the D-10 note family): every
// image-ingress failure carries a stage (config/decode/encode) and an outcome
// class, so the loud note names the outcome — never the bytes.
type ImageError struct {
	Stage  string
	Class  string
	Detail string
}

// Error implements the error interface (fixed-form: stage + class only).
func (e *ImageError) Error() string {
	return fmt.Sprintf("image ingress refused at %s: %s", e.Stage, e.Class)
}

// decodePixels is the pixel-decode seam (image.Decode by default). The
// white-box bomb test swaps it to prove config-stage refusals never decode.
//
//nolint:gochecknoglobals // a swappable test seam mirroring image.Decode's own package-level shape
var decodePixels = image.Decode

// mediaTypesByFormat maps registered decode formats to canonical media types.
//
//nolint:gochecknoglobals // a read-only value table
var mediaTypesByFormat = map[string]string{
	"jpeg": imgJPEGMedia,
	"png":  "image/png",
	"gif":  "image/gif",
	"webp": "image/webp",
}

// extensionsByMedia maps canonical media types to file extensions (the
// sha-keyed persisted names carry the format's extension).
//
//nolint:gochecknoglobals // a read-only value table
var extensionsByMedia = map[string]string{
	imgJPEGMedia: ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// ValidateAndScaleImage is the ingress tier's core (21-05, PAR-06/D-09): it
// validates media bytes against the limits and auto-downscales when they are
// exceeded — pure Go only (golang.org/x/image/draw; the CGO_ENABLED=0 build
// gate stands, Pitfall 7).
//
// The order is load-bearing (Pitfall 6): image.DecodeConfig FIRST — the
// declared dimensions, the format (magic bytes, NOT the declared mediaType —
// the decoded format wins, T-21-16), and the byte size are checked before ANY
// pixel allocation. Dims over the decode-safety ceiling refuse at the config
// stage; over the PROVIDER limits but within the ceiling → decode + downscale
// via draw.CatmullRom to the TargetLongEdge (never upscaling), re-encoding
// JPEG at quality 85 when re-encoding is required (format preserved for
// PNG/GIF when the re-encode fits the byte cap; webp re-encodes as JPEG —
// x/image has no webp encoder). The return carries the resize provenance
// (final + original dims + the original byte size + the scaled flag);
// within-limits input returns IDENTICAL bytes with didScale=false.
//
//nolint:nonamedreturns,gocritic // the plan-pinned 8-value provenance tuple
func ValidateAndScaleImage(
	data []byte, mediaType string, limits ImageLimits,
) (scaledBytes []byte, outMediaType string, w, h, origW, origH int, didScale bool, err error) {
	cfg, format, cerr := image.DecodeConfig(bytes.NewReader(data))
	if cerr != nil {
		return nil, "", 0, 0, 0, 0, false,
			&ImageError{Stage: stageConfig, Class: imgErrUndecodable, Detail: cerr.Error()}
	}

	media, ok := mediaTypesByFormat[format]
	if !ok {
		// A registered decoder produced an unmapped format — treat as the
		// undecodable class (never a silent pass with a bogus media type).
		return nil, "", 0, 0, 0, 0, false,
			&ImageError{Stage: stageConfig, Class: imgErrUndecodable, Detail: "unmapped format " + format}
	}

	origW, origH = cfg.Width, cfg.Height

	overDims := origW > limits.MaxW || origH > limits.MaxH
	overBytes := int64(len(data)) > limits.MaxBytes

	if !overDims && !overBytes {
		return data, media, origW, origH, origW, origH, false, nil
	}

	// The pixel bomb's refusal point: declared pixels over the decode-safety
	// ceiling refuse HERE — before decodePixels is ever consulted.
	if int64(origW)*int64(origH) > maxDecodePixels {
		return nil, "", 0, 0, origW, origH, false,
			&ImageError{
				Stage:  stageConfig,
				Class:  imgErrDecodeUnsafe,
				Detail: strconv.Itoa(origW) + imgScaledSep + strconv.Itoa(origH),
			}
	}

	src, _, derr := decodePixels(bytes.NewReader(data))
	if derr != nil {
		return nil, "", 0, 0, origW, origH, false,
			&ImageError{Stage: stageDecode, Class: imgErrUndecodable, Detail: derr.Error()}
	}

	w, h = fitLongEdge(origW, origH, limits.TargetLongEdge)

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)

	out, outMedia, oerr := encodeScaled(dst, media, format, limits.MaxBytes)
	if oerr != nil {
		return nil, "", 0, 0, origW, origH, false, oerr
	}

	return out, outMedia, w, h, origW, origH, true, nil
}

// fitLongEdge computes the dims that fit the target long edge, never
// upscaling (a bytes-only over-limit image re-encodes at its own dims).
//
//nolint:gocritic // two same-typed dims read cleaner unnamed
func fitLongEdge(w, h, target int) (int, int) {
	long := max(w, h)
	if long <= target || long == 0 {
		return w, h
	}

	fitW := w * target / long
	fitH := h * target / long

	if fitW < 1 {
		fitW = 1
	}

	if fitH < 1 {
		fitH = 1
	}

	return fitW, fitH
}

// encodeScaled re-encodes the scaled frame: the source format is preserved
// when its encoding fits the byte cap (PNG/GIF stay lossless), otherwise the
// frame falls to JPEG at D-09's pinned quality — and a frame that still cannot
// fit refuses with the unshrinkable class (never a silent over-cap send).
// Returns the encoded bytes plus their (possibly re-tagged) media type: a
// webp source or a bytes-driven format fallback re-tags as image/jpeg.
//
//nolint:nonamedreturns // named for the bytes/media/error triple's readability
func encodeScaled(
	img image.Image, media, format string, maxBytes int64,
) (out []byte, outMedia string, err error) {
	if format != "webp" {
		var buf bytes.Buffer

		var eerr error

		switch format {
		case "jpeg":
			eerr = jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegRescaleQuality})
		case "png":
			eerr = png.Encode(&buf, img)
		case "gif":
			eerr = gif.Encode(&buf, img, nil)
		}

		if eerr == nil && int64(buf.Len()) <= maxBytes {
			return buf.Bytes(), media, nil
		}
	}

	// JPEG fallback (webp has no x/image encoder; a preserved-format encode
	// that missed the cap re-encodes lossy).
	var buf bytes.Buffer

	eerr := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegRescaleQuality})
	if eerr != nil {
		return nil, "", &ImageError{Stage: stageEncode, Class: imgErrUnshrinkable, Detail: eerr.Error()}
	}

	if int64(buf.Len()) > maxBytes {
		return nil, "", &ImageError{
			Stage:  stageEncode,
			Class:  imgErrUnshrinkable,
			Detail: strconv.Itoa(buf.Len()) + " bytes after downscale",
		}
	}

	return buf.Bytes(), imgJPEGMedia, nil
}

// LoadImageBytes reads a Ref's bytes back for the shaper (21-05 Task 2's
// Ref reader): the Ref is an ingress-persisted path under the session
// .ass-guard images dir. A missing/unreadable Ref is the shaper's loud
// degrade (drop the block, never a dead turn).
func LoadImageBytes(ref string) ([]byte, error) {
	data, err := os.ReadFile(ref)
	if err != nil {
		return nil, fmt.Errorf("load image ref: %w", err)
	}

	return data, nil
}

// imageRefFor derives the sha-keyed file name for ingested bytes: the
// ORIGINAL persists as <sha256>.<ext>; a scaled derivative as
// <sha256>.<w>x<h>.<ext> (idempotent by construction — the same bytes always
// hash to the same name, so re-ingress converges on identical paths).
func imageRefFor(
	imagesDir string, sum [sha256.Size]byte, mediaType string, w, h int, scaled bool,
) string {
	ext := extensionsByMedia[mediaType]
	if ext == "" {
		ext = ".img"
	}

	name := hex.EncodeToString(sum[:]) + ext
	if scaled {
		name = hex.EncodeToString(sum[:]) + "." + strconv.Itoa(w) +
			imgScaledSep + strconv.Itoa(h) + ext
	}

	return filepath.Join(imagesDir, name)
}

// imgScaledSep separates the dims in a scaled derivative's file name.
const imgScaledSep = "x"

// The images tree's permission profile mirrors the transcript's
// (openTranscript's dir/file modes).
const (
	imgDirPerm  = 0o750
	imgFilePerm = 0o600
)

// writeImageAtomic persists bytes at path via temp+rename (atomic: a
// cancellation mid-write never leaves a partial Ref; a concurrent identical
// write converges on identical content).
func writeImageAtomic(imagesDir, path string, data []byte) error {
	err := os.MkdirAll(imagesDir, imgDirPerm)
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}

	tmp, err := os.CreateTemp(imagesDir, ".ingress-*")
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}

	tmpName := tmp.Name()

	_, err = tmp.Write(data)
	if err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)

		return fmt.Errorf("call: %w", err)
	}

	err = tmp.Close()
	if err != nil {
		_ = os.Remove(tmpName)

		return fmt.Errorf("call: %w", err)
	}

	err = os.Chmod(tmpName, imgFilePerm)
	if err != nil {
		_ = os.Remove(tmpName)

		return fmt.Errorf("call: %w", err)
	}

	err = os.Rename(tmpName, path)
	if err != nil {
		_ = os.Remove(tmpName)

		return fmt.Errorf("call: %w", err)
	}

	return nil
}
