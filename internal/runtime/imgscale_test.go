package runtime //nolint:testpackage // internal package test

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// 21-05 (PAR-06/D-09) image-ingress battery: ValidateAndScaleImage's six
// families (within-limits passthrough, over-dims downscale to the 1568 px long
// edge, over-bytes downscale within the cap, PNG format preservation, the
// pixel-bomb DecodeConfig-stage refusal, undecodable typed errors) plus the
// GIF/WebP acceptance rows and the ingress wiring (Ref-form rewrite, lean
// transcript lines, idempotent re-ingress, loud failure drops). All fixtures
// are synthesized in-test — no committed binary fixtures.

// Image-test fixture markers (unique so Contains-asserts cannot pass on
// fixture plumbing by accident).
const (
	imgSessionID = "sess-imgscale"
	imgJPEGMedia = "image/jpeg"
	imgPNGMedia  = "image/png"
	imgGIFMedia  = "image/gif"
	imgWebPMedia = "image/webp"
	imgTypeImage = "image"
)

// decodeCountGate serializes the white-box decodePixels swap (the bomb test's
// counting hook mutates a package var; non-parallel discipline + this mutex
// keep concurrent families safe).
var decodeCountGate sync.Mutex

// withDecodeCounter swaps the pixel-decode seam for a counting wrapper for the
// duration of one test (the pixel-bomb proof: config-stage refusals must never
// reach a decode).
func withDecodeCounter(t *testing.T) *counter { //nolint:paralleltest // mutates package var
	t.Helper()

	decodeCountGate.Lock()

	var n counter

	old := decodePixels
	decodePixels = func(r io.Reader) (image.Image, string, error) {
		n.mu.Lock()
		n.decodes++
		n.mu.Unlock()

		return old(r)
	}

	t.Cleanup(func() {
		decodePixels = old
		decodeCountGate.Unlock()
	})

	return &n
}

// counter is the decode-count observation sink.
type counter struct {
	mu      sync.Mutex
	decodes int
}

func (c *counter) get() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.decodes
}

// gradient fills an RGBA image with a deterministic diagonal gradient
// (compressible for JPEG/PNG, deterministic across runs).
func gradient(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))

	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8((x + y) % 256),
				G: uint8((x * 3) % 256),
				B: uint8((y * 5) % 256),
				A: 255,
			})
		}
	}

	return img
}

// encodeJPEG renders the gradient as a JPEG of the given dims.
func encodeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()

	var buf bytes.Buffer

	err := jpeg.Encode(&buf, gradient(w, h), &jpeg.Options{Quality: 90})
	if err != nil {
		t.Fatalf("encode jpeg %dx%d: %v", w, h, err)
	}

	return buf.Bytes()
}

// encodePNG renders the gradient as a PNG of the given dims.
func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()

	var buf bytes.Buffer

	err := png.Encode(&buf, gradient(w, h))
	if err != nil {
		t.Fatalf("encode png %dx%d: %v", w, h, err)
	}

	return buf.Bytes()
}

// encodeNoisePNG renders an incompressible random-ish noise PNG (poor
// compression forces the over-bytes class at small dims).
func encodeNoisePNG(t *testing.T, w, h int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	seed := uint32(0x9E3779B9)

	for y := range h {
		for x := range w {
			seed = seed*1664525 + 1013904223 //nolint:mnd // LCG noise
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(seed >> 24),
				G: uint8(seed >> 16),
				B: uint8(seed >> 8),
				A: 255,
			})
		}
	}

	var buf bytes.Buffer

	err := png.Encode(&buf, img)
	if err != nil {
		t.Fatalf("encode noise png %dx%d: %v", w, h, err)
	}

	return buf.Bytes()
}

// encodeGIF renders a small paletted GIF.
func encodeGIF(t *testing.T, w, h int) []byte {
	t.Helper()

	img := image.NewPaletted(image.Rect(0, 0, w, h), color.Palette{
		color.RGBA{R: 0, G: 0, B: 0, A: 255},
		color.RGBA{R: 255, G: 0, B: 0, A: 255},
		color.RGBA{R: 0, G: 255, B: 0, A: 255},
	})

	for y := range h {
		for x := range w {
			img.SetColorIndex(x, y, uint8((x+y)%3))
		}
	}

	var buf bytes.Buffer

	err := gif.Encode(&buf, img, nil)
	if err != nil {
		t.Fatalf("encode gif %dx%d: %v", w, h, err)
	}

	return buf.Bytes()
}

// craftWebPHeader builds a header-valid VP8L WEBP (RIFF container + VP8L chunk
// carrying the dims; x/image has no webp ENCODER, so the fixture is the
// header craft — DecodeConfig reads the dims without the pixel payload).
func craftWebPHeader(w, h int) []byte {
	payload := []byte{0x2F} // VP8L signature

	bits := uint32(w-1) | uint32(h-1)<<14 //nolint:mnd // VP8L packs 14 bits per dim
	var packed [4]byte

	binary.LittleEndian.PutUint32(packed[:], bits)
	payload = append(payload, packed[:]...)

	chunk := append([]byte("VP8L"), byte(len(payload)), 0, 0, 0)
	chunk = append(chunk, payload...)

	out := []byte("RIFF")

	var sz [4]byte

	binary.LittleEndian.PutUint32(sz[:], uint32(4+len(chunk)))
	out = append(out, sz[:]...)
	out = append(out, "WEBP"...)
	out = append(out, chunk...)

	return out
}

// craftPNGBomb builds a tiny PNG whose IHDR declares w×h but carries NO pixel
// data — the pixel-bomb class (DecodeConfig reports the declared dims; a
// decode-first pipeline would allocate the full frame).
func craftPNGBomb(w, h int) []byte {
	var ihdr []byte

	ihdr = binary.BigEndian.AppendUint32(ihdr, uint32(w))
	ihdr = binary.BigEndian.AppendUint32(ihdr, uint32(h))
	ihdr = append(ihdr, 8, 0, 0, 0, 0) // bit depth 8, grayscale, no interlace

	chunk := append([]byte("IHDR"), ihdr...)
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(chunk))

	out := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	out = binary.BigEndian.AppendUint32(out, uint32(len(ihdr)))

	return append(out, chunk...)
}

// --- Family 1+4: within-limits passthrough (JPEG + PNG, format preserved) ---

// TestValidateAndScale_WithinLimitsPassthrough proves the D-09 passthrough: a
// within-limits image needs NO scaling — identical bytes, scaled=false, and
// the decoded format (not the declared media type) wins (T-21-16).
func TestValidateAndScale_WithinLimitsPassthrough(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		data         []byte
		declared     string
		wantMedia    string
		wantW, wantH int
	}{
		{name: "jpeg 800x600", data: encodeJPEG(t, 800, 600), declared: imgPNGMedia, wantMedia: imgJPEGMedia, wantW: 800, wantH: 600},
		{name: "png 640x480", data: encodePNG(t, 640, 480), declared: imgPNGMedia, wantMedia: imgPNGMedia, wantW: 640, wantH: 480},
	}

	for _, c := range cases { //nolint:paralleltest // shared decodePixels-free path
		t.Run(c.name, func(t *testing.T) {
			scaled, media, w, h, origW, origH, didScale, err := ValidateAndScaleImage(c.data, c.declared, DefaultImageLimits)
			if err != nil {
				t.Fatalf("ValidateAndScaleImage: %v", err)
			}

			if didScale {
				t.Error("didScale = true for a within-limits image; want false (no re-encode)")
			}

			if !bytes.Equal(scaled, c.data) {
				t.Error("within-limits bytes were re-encoded; want identical passthrough")
			}

			if media != c.wantMedia {
				t.Errorf("media = %q; want %q (decoded format wins over the declared %q)", media, c.wantMedia, c.declared)
			}

			if w != c.wantW || h != c.wantH || origW != c.wantW || origH != c.wantH {
				t.Errorf("dims = (%d,%d) orig (%d,%d); want (%d,%d) both", w, h, origW, origH, c.wantW, c.wantH)
			}
		})
	}
}

// --- Family 2: over-dims downscale fits the 1568 px long edge ---

// TestValidateAndScale_OverDimsDownscalesToTargetLongEdge proves D-09's
// auto-downscale: a 9000×6000 JPEG (over the 8000×8000 provider cap)
// downscale-fits the 1568 px long edge via the pure-Go scaler, records the
// resize provenance, and the output decodes at the scaled dims.
func TestValidateAndScale_OverDimsDownscalesToTargetLongEdge(t *testing.T) { //nolint:paralleltest // 216MB fixture
	data := encodeJPEG(t, 9000, 6000)

	scaled, media, w, h, origW, origH, didScale, err := ValidateAndScaleImage(data, imgJPEGMedia, DefaultImageLimits)
	if err != nil {
		t.Fatalf("ValidateAndScaleImage: %v", err)
	}

	if !didScale {
		t.Fatal("didScale = false for an over-dims image; want true")
	}

	if origW != 9000 || origH != 6000 {
		t.Errorf("provenance orig dims = (%d,%d); want (9000,6000)", origW, origH)
	}

	// 1568 long edge: 9000 → 1568, 6000 → 1045 (rounding down keeps within).
	if w != DefaultImageLimits.TargetLongEdge {
		t.Errorf("scaled width = %d; want %d (the target long edge)", w, DefaultImageLimits.TargetLongEdge)
	}

	if h != 1045 { //nolint:mnd // 6000 * 1568 / 9000
		t.Errorf("scaled height = %d; want 1045", h)
	}

	if media != imgJPEGMedia {
		t.Errorf("media = %q; want %q (JPEG re-encoded at quality 85)", media, imgJPEGMedia)
	}

	cfg, format, derr := image.DecodeConfig(bytes.NewReader(scaled))
	if derr != nil {
		t.Fatalf("scaled output does not decode: %v", derr)
	}

	if format != "jpeg" || cfg.Width != w || cfg.Height != h {
		t.Errorf("scaled output config = %s %dx%d; want jpeg %dx%d", format, cfg.Width, cfg.Height, w, h)
	}
}

// --- Family 3: over-bytes downscale lands within the cap ---

// TestValidateAndScale_OverBytesDownscalesWithinCap proves the byte-cap leg:
// a noise PNG within dims but over the byte cap downscales until the
// re-encoded output fits the cap (custom small limits keep the fixture cheap;
// the code path is the production one).
func TestValidateAndScale_OverBytesDownscalesWithinCap(t *testing.T) {
	t.Parallel()

	limits := ImageLimits{MaxW: 8000, MaxH: 8000, MaxBytes: 50 << 10, TargetLongEdge: 200} //nolint:mnd // cheap fixture scale

	data := encodeNoisePNG(t, 400, 400) //nolint:mnd // ~330 KB PNG, dims within, bytes over

	if int64(len(data)) <= limits.MaxBytes {
		t.Fatalf("fixture too small (%d bytes) to exceed the %d-byte cap", len(data), limits.MaxBytes)
	}

	scaled, media, w, h, _, _, didScale, err := ValidateAndScaleImage(data, imgPNGMedia, limits)
	if err != nil {
		t.Fatalf("ValidateAndScaleImage: %v", err)
	}

	if !didScale {
		t.Fatal("didScale = false for an over-bytes image; want true")
	}

	if int64(len(scaled)) > limits.MaxBytes {
		t.Errorf("scaled output = %d bytes; want <= %d (downscales until within the cap)", len(scaled), limits.MaxBytes)
	}

	if w != limits.TargetLongEdge {
		t.Errorf("scaled long edge = %d; want %d", w, limits.TargetLongEdge)
	}

	if media != imgPNGMedia {
		t.Errorf("media = %q; want %q (format preserved when the re-encode fits)", media, imgPNGMedia)
	}
}

// --- Family 5: the pixel bomb — refused at the DecodeConfig stage ---

// TestValidateAndScale_PixelBombRefusedBeforeDecode proves Pitfall 6: a tiny
// PNG whose IHDR declares 40000×40000 is REFUSED on its declared dimensions
// BEFORE any pixel decode — the counting seam observes ZERO decodes, and the
// typed error reports the config stage.
func TestValidateAndScale_PixelBombRefusedBeforeDecode(t *testing.T) { //nolint:paralleltest // swaps package var
	counts := withDecodeCounter(t)

	bomb := craftPNGBomb(40000, 40000)

	if len(bomb) > 256 { //nolint:mnd // the bomb IS tiny — a big fixture means the craft broke
		t.Fatalf("bomb fixture = %d bytes; want a tiny header-only file", len(bomb))
	}

	_, _, _, _, _, _, _, err := ValidateAndScaleImage(bomb, imgPNGMedia, DefaultImageLimits)
	if err == nil {
		t.Fatal("pixel bomb accepted; want a loud refusal")
	}

	var imgErr *ImageError

	if !errors.As(err, &imgErr) {
		t.Fatalf("error %v is not *ImageError; want the typed ingress-failure class", err)
	}

	if imgErr.Stage != stageConfig {
		t.Errorf("refusal stage = %q; want %q (DecodeConfig-stage, before any pixel allocation)", imgErr.Stage, stageConfig)
	}

	if got := counts.get(); got != 0 {
		t.Fatalf("pixel decode ran %d time(s) on the bomb path; want 0 (refusal must precede decode)", got)
	}
}

// --- Family 6: undecodable bytes — typed error, never a panic ---

// TestValidateAndScale_UndecodableBytesTypedError proves the loud-note class:
// random data and unknown formats produce the typed error (never a panic),
// reported at the config stage.
func TestValidateAndScale_UndecodableBytesTypedError(t *testing.T) {
	t.Parallel()

	junk := []byte("definitely not an image, just bytes 0123456789")

	_, _, _, _, _, _, _, err := ValidateAndScaleImage(junk, imgPNGMedia, DefaultImageLimits)
	if err == nil {
		t.Fatal("undecodable bytes accepted; want a typed error")
	}

	var imgErr *ImageError

	if !errors.As(err, &imgErr) {
		t.Fatalf("error %v is not *ImageError", err)
	}

	if imgErr.Stage != stageConfig || imgErr.Class != imgErrUndecodable {
		t.Errorf("error = stage %q class %q; want stage %q class %q",
			imgErr.Stage, imgErr.Class, stageConfig, imgErrUndecodable)
	}
}

// --- GIF + WebP acceptance ---

// TestValidateAndScale_GIFAndWebPAccepted proves the D-09 media-type set:
// stdlib gif decodes through the pipeline, and webp (x/image registration)
// is ACCEPTED — DecodeConfig reads the crafted VP8L header's dims.
func TestValidateAndScale_GIFAndWebPAccepted(t *testing.T) {
	t.Parallel()

	gifData := encodeGIF(t, 60, 40)

	_, media, w, h, _, _, didScale, err := ValidateAndScaleImage(gifData, imgGIFMedia, DefaultImageLimits)
	if err != nil {
		t.Fatalf("gif: %v", err)
	}

	if didScale || media != imgGIFMedia || w != 60 || h != 40 { //nolint:mnd // fixture dims
		t.Errorf("gif = media %q %dx%d scaled=%v; want passthrough %q 60x40", media, w, h, didScale, imgGIFMedia)
	}

	webpData := craftWebPHeader(61, 41) //nolint:mnd // fixture dims

	_, media, w, h, _, _, didScale, err = ValidateAndScaleImage(webpData, imgWebPMedia, DefaultImageLimits)
	if err != nil {
		t.Fatalf("webp: %v (registration or acceptance broke)", err)
	}

	if didScale || media != imgWebPMedia || w != 61 || h != 41 {
		t.Errorf("webp = media %q %dx%d scaled=%v; want passthrough %q 61x41", media, w, h, didScale, imgWebPMedia)
	}
}

// --- Ingress wiring (runtime seam) ---

// newImageRunner builds a Runner + minimal session over a temp workspace
// (Manager only — the ingress writes next to the transcript and nothing else
// of the session is consulted).
func newImageRunner(t *testing.T) (r *Runner, sess *session.Session, dir string) {
	t.Helper()

	dir = t.TempDir()

	mgr, err := session.NewManager(dir, imgSessionID, redactorAdapter{})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	t.Cleanup(func() { _ = mgr.Close() })

	return &Runner{workDir: dir}, &session.Session{Manager: mgr}, dir
}

// imagePromptBlocks builds the acp-shaped prompt: one text block + one image
// block carrying base64 data.
func imagePromptBlocks(b64 string) []acp.ContentBlock {
	return []acp.ContentBlock{
		{Type: blockText, Text: "what is in this picture?"},
		{Type: imgTypeImage, Data: b64, MimeType: imgPNGMedia},
	}
}

// TestImageIngress_MapsAndRewritesToRefForm proves the ingress tier: acp image
// blocks map through toContentBlocks into session blocks, ingress validates +
// persists, and the rewritten block carries Ref + metadata + provenance —
// NEVER the base64 payload.
func TestImageIngress_MapsAndRewritesToRefForm(t *testing.T) {
	t.Parallel()

	r, sess, dir := newImageRunner(t)

	pngData := encodePNG(t, 320, 200)
	b64 := base64.StdEncoding.EncodeToString(pngData)

	acpBlocks := imagePromptBlocks(b64)
	blocks := toContentBlocks(acpBlocks)

	if blocks[1].Type != imgTypeImage || blocks[1].Data != b64 || blocks[1].MimeType != imgPNGMedia {
		t.Fatalf("toContentBlocks dropped the image fields: %+v", blocks[1])
	}

	out := r.ingressImages(sess, blocks)
	if len(out) != 2 {
		t.Fatalf("ingress returned %d blocks; want 2 (text + rewritten image)", len(out))
	}

	if out[0].Type != blockText || out[0].Text != "what is in this picture?" {
		t.Errorf("text block mutated by ingress: %+v", out[0])
	}

	img := out[1]
	if img.Data != "" {
		t.Error("rewritten block still carries base64 Data; want it dropped (09-05 discipline)")
	}

	if img.DataRef == "" || img.MediaType != imgPNGMedia {
		t.Errorf("rewritten block = dataRef %q media %q; want a Ref + %q", img.DataRef, img.MediaType, imgPNGMedia)
	}

	if img.Width != 320 || img.Height != 200 || img.OrigWidth != 320 || img.OrigHeight != 200 { //nolint:mnd // fixture dims
		t.Errorf("metadata dims = (%d,%d) orig (%d,%d); want (320,200) both", img.Width, img.Height, img.OrigWidth, img.OrigHeight)
	}

	if img.Scaled {
		t.Error("Scaled = true for a within-limits image; want false")
	}

	if img.OrigSize != int64(len(pngData)) {
		t.Errorf("OrigSize = %d; want %d", img.OrigSize, len(pngData))
	}

	// The Ref'd file exists on disk with the ORIGINAL bytes.
	onDisk, rerr := os.ReadFile(img.DataRef)
	if rerr != nil {
		t.Fatalf("Ref'd file unreadable: %v", rerr)
	}

	if !bytes.Equal(onDisk, pngData) {
		t.Error("Ref'd file bytes differ from the original payload")
	}

	if !strings.HasPrefix(img.DataRef, filepath.Join(dir, ".ass-guard")) {
		t.Errorf("Ref %q does not live under the session .ass-guard dir", img.DataRef)
	}
}

// TestImageIngress_PersistsOriginalAndScaled proves D-09's originals-preserved
// contract on the downscale path: BOTH the sha-keyed original and the
// dims-suffixed scaled file land on disk, original bytes byte-equal the input.
func TestImageIngress_PersistsOriginalAndScaled(t *testing.T) {
	t.Parallel()

	r, sess, _ := newImageRunner(t)
	r.imageLimits = ImageLimits{MaxW: 200, MaxH: 200, MaxBytes: 10 << 20, TargetLongEdge: 100} //nolint:mnd // cheap fixture scale

	pngData := encodePNG(t, 400, 300)
	b64 := base64.StdEncoding.EncodeToString(pngData)

	out := r.ingressImages(sess, toContentBlocks(imagePromptBlocks(b64)))
	if len(out) != 2 {
		t.Fatalf("ingress returned %d blocks; want 2", len(out))
	}

	img := out[1]

	if !img.Scaled {
		t.Fatal("Scaled = false for an over-limit image; want true")
	}

	if img.Width != 100 || img.Height != 75 { //nolint:mnd // 300*100/400
		t.Errorf("scaled dims = (%d,%d); want (100,75)", img.Width, img.Height)
	}

	if img.OrigWidth != 400 || img.OrigHeight != 300 || img.OrigSize != int64(len(pngData)) { //nolint:mnd // fixture dims
		t.Errorf("provenance = orig (%d,%d) size %d; want (400,300) size %d",
			img.OrigWidth, img.OrigHeight, img.OrigSize, len(pngData))
	}

	// The Ref points at the SCALED file; the ORIGINAL lives beside it.
	scaledBytes, serr := os.ReadFile(img.DataRef)
	if serr != nil {
		t.Fatalf("scaled Ref unreadable: %v", serr)
	}

	cfg, _, derr := image.DecodeConfig(bytes.NewReader(scaledBytes))
	if derr != nil || cfg.Width != 100 || cfg.Height != 75 { //nolint:mnd // fixture dims
		t.Errorf("scaled file config = %dx%d err %v; want 100x75", cfg.Width, cfg.Height, derr)
	}

	dir := filepath.Dir(img.DataRef)
	scaledBase := filepath.Base(img.DataRef)

	entries, aerr := os.ReadDir(dir)
	if aerr != nil {
		t.Fatalf("ReadDir(images): %v", aerr)
	}

	var origName string

	for _, e := range entries {
		if e.Name() != scaledBase {
			origName = e.Name()
		}
	}

	if origName == "" {
		t.Fatal("no second (original) file beside the scaled Ref; want <sha>.<ext> + <sha>.<w>x<h>.<ext>")
	}

	if !strings.Contains(scaledBase, "100x75") { //nolint:mnd // fixture dims
		t.Errorf("scaled file name %q lacks the dims suffix; want the <sha>.<w>x<h>.<ext> form", scaledBase)
	}

	origBytes, oerr := os.ReadFile(filepath.Join(dir, origName))
	if oerr != nil {
		t.Fatalf("original file unreadable: %v", oerr)
	}

	if !bytes.Equal(origBytes, pngData) {
		t.Error("on-disk original bytes differ from the ingested payload (originals must be preserved)")
	}
}

// TestImageIngress_TranscriptLeanNoBase64 proves the privacy prohibition: the
// appended user_message line carries Ref + metadata + provenance and NO
// base64 payload.
func TestImageIngress_TranscriptLeanNoBase64(t *testing.T) {
	t.Parallel()

	r, sess, _ := newImageRunner(t)

	pngData := encodePNG(t, 320, 200)
	b64 := base64.StdEncoding.EncodeToString(pngData)

	out := r.ingressImages(sess, toContentBlocks(imagePromptBlocks(b64)))

	if err := sess.Manager.AppendUserMessage("turn_1", out); err != nil {
		t.Fatalf("AppendUserMessage: %v", err)
	}

	raw, rerr := os.ReadFile(sess.Manager.Path())
	if rerr != nil {
		t.Fatalf("read transcript: %v", rerr)
	}

	body := string(raw)

	if strings.Contains(body, b64) {
		t.Error("transcript contains the base64 payload; want Ref-only lines (no image bytes in the transcript)")
	}

	if !strings.Contains(body, `"dataRef"`) || !strings.Contains(body, `"mediaType"`) {
		t.Error("transcript line lacks the image metadata fields (dataRef/mediaType)")
	}

	if !strings.Contains(body, `"origWidth"`) || !strings.Contains(body, `"origSize"`) {
		t.Error("transcript line lacks the resize provenance fields (origWidth/origSize)")
	}

	// The line still round-trips through the typed reader.
	lines, lerr := sess.Manager.ReadAll()
	if lerr != nil {
		t.Fatalf("ReadAll: %v", lerr)
	}

	var blocks []session.ContentBlock

	if err := json.Unmarshal(lines[len(lines)-1].Content, &blocks); err != nil {
		t.Fatalf("unmarshal user_message content: %v", err)
	}

	if len(blocks) != 2 || blocks[1].DataRef == "" || blocks[1].Data != "" {
		t.Errorf("round-tripped blocks = %+v; want text + Ref-form image", blocks)
	}
}

// TestImageIngress_IdempotentReIngress proves the flagged concurrency row: the
// same image ingested twice (fresh session state) converges on identical Ref
// paths and bytes — the second write is a no-op onto identical content.
func TestImageIngress_IdempotentReIngress(t *testing.T) {
	t.Parallel()

	r, sess, _ := newImageRunner(t)

	pngData := encodePNG(t, 320, 200)
	b64 := base64.StdEncoding.EncodeToString(pngData)

	first := r.ingressImages(sess, toContentBlocks(imagePromptBlocks(b64)))

	// Fresh session state over the SAME workspace (a second session in the
	// same dir re-derives the same sha-keyed Refs).
	mgr2, err := session.NewManager(filepath.Dir(sess.Manager.Path()), imgSessionID+"-2", redactorAdapter{})
	if err != nil {
		t.Fatalf("NewManager(2): %v", err)
	}

	t.Cleanup(func() { _ = mgr2.Close() })

	sess2 := &session.Session{Manager: mgr2}

	second := r.ingressImages(sess2, toContentBlocks(imagePromptBlocks(b64)))

	if first[1].DataRef != second[1].DataRef {
		t.Errorf("Refs diverged across ingresses: %q vs %q; want identical (sha-keyed convergence)",
			first[1].DataRef, second[1].DataRef)
	}

	a, _ := os.ReadFile(first[1].DataRef)
	b, _ := os.ReadFile(second[1].DataRef)

	if !bytes.Equal(a, b) {
		t.Error("Ref'd bytes diverged across ingresses; want identical")
	}
}

// TestImageIngress_FailureDropsBlockWithLoudNote proves the D-10-family
// degrade: an undecodable image block is DROPPED with exactly one fixed-form
// stderr note naming the outcome class — the text block survives, the turn
// proceeds, and no bytes leak into the note.
func TestImageIngress_FailureDropsBlockWithLoudNote(t *testing.T) {
	t.Parallel()

	r, sess, _ := newImageRunner(t)

	var notes bytes.Buffer

	r.stderr = &notes

	junk := []byte("not an image at all")

	blocks := toContentBlocks(imagePromptBlocks(base64.StdEncoding.EncodeToString(junk)))

	out := r.ingressImages(sess, blocks)
	if len(out) != 1 {
		t.Fatalf("ingress returned %d blocks; want 1 (image dropped, text kept)", len(out))
	}

	if out[0].Type != blockText {
		t.Errorf("surviving block = %+v; want the text block", out[0])
	}

	note := notes.String()
	if !strings.Contains(note, "image") {
		t.Errorf("stderr note = %q; want a loud note naming the image drop", note)
	}

	if strings.Contains(note, string(junk)) {
		t.Error("stderr note leaks image bytes; want outcome-class wording only")
	}
}
