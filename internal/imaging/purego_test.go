//go:build !libvips || !cgo

package imaging_test

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"testing"

	"github.com/AXmishell/axmipic/internal/imaging"
)

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

func TestInfo(t *testing.T) {
	processor := imaging.Default()
	info, err := processor.Info(testPNG(t, 12, 7))
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Width != 12 || info.Height != 7 || info.Format != imaging.FormatPNG {
		t.Fatalf("info = %+v, want 12x7 png", info)
	}
}

func TestProcessContainResize(t *testing.T) {
	processor := imaging.Default()
	result, err := processor.Process(testPNG(t, 8, 8), imaging.Options{
		Width: 4, Fit: imaging.FitContain, Quality: 82, Format: imaging.FormatPNG, StripMetadata: true,
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if result.ContentType != "image/png" {
		t.Fatalf("content type = %q", result.ContentType)
	}
	if result.Width != 4 || result.Height != 4 {
		t.Fatalf("result = %dx%d, want 4x4", result.Width, result.Height)
	}
}

func TestProcessDoesNotEnlargeByDefault(t *testing.T) {
	processor := imaging.Default()
	result, err := processor.Process(testPNG(t, 4, 4), imaging.Options{
		Width: 32, Height: 32, Fit: imaging.FitContain, Quality: 82, Format: imaging.FormatPNG, StripMetadata: true,
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if result.Width != 4 || result.Height != 4 {
		t.Fatalf("result = %dx%d, want unchanged 4x4", result.Width, result.Height)
	}
}

func TestProcessCoverCrop(t *testing.T) {
	processor := imaging.Default()
	result, err := processor.Process(testPNG(t, 8, 8), imaging.Options{
		Width: 4, Height: 2, Fit: imaging.FitCover, Quality: 82, Format: imaging.FormatPNG, StripMetadata: true,
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if result.Width != 4 || result.Height != 2 {
		t.Fatalf("result = %dx%d, want 4x2", result.Width, result.Height)
	}
}

func TestProcessConvertsToJPEG(t *testing.T) {
	processor := imaging.Default()
	result, err := processor.Process(testPNG(t, 8, 8), imaging.Options{
		Fit: imaging.FitContain, Quality: 80, Format: imaging.FormatJPEG, StripMetadata: true,
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if result.ContentType != "image/jpeg" || result.Format != imaging.FormatJPEG {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(result.Data) == 0 {
		t.Fatal("empty output")
	}
}

func TestProcessRejectsUnsupportedFormat(t *testing.T) {
	processor := imaging.Default()
	_, err := processor.Process(testPNG(t, 8, 8), imaging.Options{
		Fit: imaging.FitContain, Quality: 80, Format: imaging.FormatWebP, StripMetadata: true,
	})
	if !errors.Is(err, imaging.ErrUnsupportedFormat) {
		t.Fatalf("error = %v, want ErrUnsupportedFormat", err)
	}
}

func TestProcessRejectsInvalidInput(t *testing.T) {
	processor := imaging.Default()
	_, err := processor.Process([]byte("definitely not an image"), imaging.Options{
		Fit: imaging.FitContain, Quality: 80, Format: imaging.FormatPNG, StripMetadata: true,
	})
	if !errors.Is(err, imaging.ErrDecode) {
		t.Fatalf("error = %v, want ErrDecode", err)
	}
}

func TestProcessGrayscaleAndFlip(t *testing.T) {
	processor := imaging.Default()
	result, err := processor.Process(testPNG(t, 6, 4), imaging.Options{
		Fit: imaging.FitContain, Quality: 82, Format: imaging.FormatPNG,
		StripMetadata: true, Grayscale: true, Flip: "h",
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if result.Width != 6 || result.Height != 4 {
		t.Fatalf("result = %dx%d, want 6x4", result.Width, result.Height)
	}
}

func TestProcessBlurSharpen(t *testing.T) {
	processor := imaging.Default()
	if _, err := processor.Process(testPNG(t, 8, 8), imaging.Options{
		Fit: imaging.FitContain, Quality: 82, Format: imaging.FormatPNG,
		StripMetadata: true, Blur: 2, Sharpen: 1,
	}); err != nil {
		t.Fatalf("Process: %v", err)
	}
}

func TestProcessFillStretch(t *testing.T) {
	processor := imaging.Default()
	result, err := processor.Process(testPNG(t, 8, 4), imaging.Options{
		Width: 4, Height: 8, Fit: imaging.FitFill, Quality: 82, Format: imaging.FormatPNG, StripMetadata: true,
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if result.Width != 4 || result.Height != 8 {
		t.Fatalf("result = %dx%d, want 4x8", result.Width, result.Height)
	}
}

func TestProcessWatermark(t *testing.T) {
	processor := imaging.Default()
	result, err := processor.Process(testPNG(t, 64, 64), imaging.Options{
		Fit: imaging.FitContain, Quality: 82, Format: imaging.FormatPNG, StripMetadata: true,
		Watermark: &imaging.Watermark{Text: "AXmiPic", Position: "bottom-right", Opacity: 70},
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(result.Data) == 0 {
		t.Fatal("empty output")
	}
}

func TestProcessRejectsEmptyWatermark(t *testing.T) {
	processor := imaging.Default()
	_, err := processor.Process(testPNG(t, 8, 8), imaging.Options{
		Fit: imaging.FitContain, Quality: 80, Format: imaging.FormatPNG, StripMetadata: true,
		Watermark: &imaging.Watermark{Text: "   "},
	})
	if !errors.Is(err, imaging.ErrInvalidOptions) {
		t.Fatalf("error = %v, want ErrInvalidOptions", err)
	}
}

func TestParseFlipAndFill(t *testing.T) {
	if f, ok := imaging.ParseFit("stretch"); !ok || f != imaging.FitFill {
		t.Fatalf("ParseFit(stretch) = %q, %v", f, ok)
	}
	if v, ok := imaging.ParseFlip("both"); !ok || v != "hv" {
		t.Fatalf("ParseFlip(both) = %q, %v", v, ok)
	}
	if _, ok := imaging.ParseFlip("diagonal"); ok {
		t.Fatalf("ParseFlip(diagonal) should fail")
	}
}
