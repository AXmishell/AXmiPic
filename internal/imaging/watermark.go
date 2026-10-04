package imaging

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// watermarkPosition 返回水印相对于画布的锚点坐标（水印左上角）。
func watermarkPosition(canvasW, canvasH, wmW, wmH int, position string) (int, int) {
	margin := int(math.Max(6, float64(canvasW)*0.015))
	switch strings.ToLower(strings.TrimSpace(position)) {
	case "top-left":
		return margin, margin
	case "top-right":
		return canvasW - wmW - margin, margin
	case "bottom-left":
		return margin, canvasH - wmH - margin
	case "center":
		return (canvasW - wmW) / 2, (canvasH - wmH) / 2
	case "bottom-right", "":
		fallthrough
	default:
		return canvasW - wmW - margin, canvasH - wmH - margin
	}
}

// parseHexColor 解析 #rgb/#rrggbb 颜色；无效时返回白色。
func parseHexColor(value string) color.RGBA {
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	switch len(value) {
	case 3:
		value = string([]byte{value[0], value[0], value[1], value[1], value[2], value[2]})
	case 6:
	default:
		return color.RGBA{R: 255, G: 255, B: 255, A: 255}
	}
	var rgb [3]uint8
	for i := 0; i < 3; i++ {
		high := hexVal(value[i*2])
		low := hexVal(value[i*2+1])
		if high < 0 || low < 0 {
			return color.RGBA{R: 255, G: 255, B: 255, A: 255}
		}
		rgb[i] = uint8(high<<4 | low)
	}
	return color.RGBA{R: rgb[0], G: rgb[1], B: rgb[2], A: 255}
}

// hexVal 返回十六进制字符的值，无效时返回 -1。
func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	default:
		return -1
	}
}

// drawTextWatermark 在 dst 上绘制文字水印。fontSize 为 0 时按画布尺寸自适应。
// 该函数与具体处理器无关，供纯 Go 与 libvips 之前的准备阶段共享。
func drawTextWatermark(dst *image.RGBA, wm Watermark) {
	if dst == nil || strings.TrimSpace(wm.Text) == "" {
		return
	}
	bounds := dst.Bounds()
	fontSize := wm.Size
	if fontSize <= 0 {
		fontSize = int(math.Max(12, float64(bounds.Dy())*0.05))
	}
	face, err := newWatermarkFace(fontSize)
	if err != nil {
		return
	}
	defer func() { _ = face.Close() }()

	textWidth := font.MeasureString(face, wm.Text).Ceil()
	metrics := face.Metrics()
	ascent := metrics.Ascent.Ceil()
	descent := metrics.Descent.Ceil()
	textHeight := ascent + descent

	x, y := watermarkPosition(bounds.Dx(), bounds.Dy(), textWidth, textHeight, wm.Position)
	opacity := wm.Opacity
	if opacity <= 0 {
		opacity = 80
	}
	if opacity > 100 {
		opacity = 100
	}
	base := parseHexColor(wm.Color)
	// 使用低透明度颜色叠加，形成半透明水印。
	glyphColor := color.RGBA{R: base.R, G: base.G, B: base.B, A: uint8(opacity * 255 / 100)}

	// 先绘制一层深色阴影，保证浅色背景上依然可辨认。
	shadow := color.RGBA{R: 0, G: 0, B: 0, A: glyphColor.A / 2}
	drawer := &font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(shadow),
		Face: face,
		Dot:  fixed.P(x+1, y+ascent+1),
	}
	drawer.DrawString(wm.Text)
	drawer.Src = image.NewUniform(glyphColor)
	drawer.Dot = fixed.P(x, y+ascent)
	drawer.DrawString(wm.Text)
}

// newWatermarkFace 基于 Go Bold 字体构造指定字号的字体面。
func newWatermarkFace(size int) (font.Face, error) {
	parsed, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(parsed, &opentype.FaceOptions{
		Size:    float64(size),
		DPI:     72,
		Hinting: font.HintingFull,
	})
}

// applyRGBA 把任意图像转换为可写的 RGBA 以确保有稳定的像素格式。
func toRGBA(img image.Image) *image.RGBA {
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba
	}
	bounds := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(dst, dst.Bounds(), img, bounds.Min, draw.Src)
	return dst
}
