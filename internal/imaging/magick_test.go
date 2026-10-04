//go:build !libvips || !cgo

package imaging

import (
	"strings"
	"testing"
)

func TestBuildMagickArgsCover(t *testing.T) {
	args := buildMagickArgs(Options{
		Width: 100, Height: 50, Fit: FitCover,
		Quality: 80, Format: FormatJPEG, StripMetadata: true,
	})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-resize 100x50^") {
		t.Fatalf("missing cover resize: %s", joined)
	}
	if !strings.Contains(joined, "-extent 100x50") {
		t.Fatalf("missing extent: %s", joined)
	}
	if !strings.Contains(joined, "jpeg:") {
		t.Fatalf("missing format: %s", joined)
	}
}

func TestBuildMagickArgsEffects(t *testing.T) {
	args := buildMagickArgs(Options{
		Fit: FitContain, Quality: 82, Format: FormatPNG,
		Grayscale: true, Blur: 3, Sharpen: 2, Flip: "hv", Rotate: 90,
	})
	joined := strings.Join(args, " ")
	for _, want := range []string{"-colorspace Gray", "-blur", "-sharpen", "-flip", "-flop", "-rotate 90", "png:"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in: %s", want, joined)
		}
	}
}

func TestDetectOutputFormat(t *testing.T) {
	if f, _ := detectOutputFormat([]byte("\xFF\xD8\xFF\xE0"), ""); f != FormatJPEG {
		t.Fatalf("jpeg detect = %q", f)
	}
	if f, _ := detectOutputFormat([]byte("\x89PNG\r\n"), ""); f != FormatPNG {
		t.Fatalf("png detect = %q", f)
	}
	if f, _ := detectOutputFormat([]byte("RIFFxxxxWEBPVP8 "), ""); f != FormatWebP {
		t.Fatalf("webp detect = %q", f)
	}
	if f, _ := detectOutputFormat([]byte("anything"), FormatGIF); f != FormatGIF {
		t.Fatalf("explicit format = %q", f)
	}
}
