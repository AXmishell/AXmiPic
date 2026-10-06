package imaging

import (
	"strconv"
	"strings"
)

// OptionsDigest 返回一组变换选项的确定性、可比较表示。它用于为派生图构建
// 缓存键与 ETag；与 fmt 的默认格式化不同，这里显式列出每个字段，因此不会
// 受指针地址或字段新增/重排的影响。
func OptionsDigest(opts Options) string {
	var b strings.Builder
	b.Grow(96)
	b.WriteString("w=")
	b.WriteString(strconv.Itoa(opts.Width))
	b.WriteString(";h=")
	b.WriteString(strconv.Itoa(opts.Height))
	b.WriteString(";fit=")
	b.WriteString(string(opts.Fit))
	b.WriteString(";q=")
	b.WriteString(strconv.Itoa(opts.Quality))
	b.WriteString(";fmt=")
	b.WriteString(string(opts.Format))
	b.WriteString(";strip=")
	b.WriteString(strconv.FormatBool(opts.StripMetadata))
	b.WriteString(";enlarge=")
	b.WriteString(strconv.FormatBool(opts.Enlarge))
	b.WriteString(";rot=")
	b.WriteString(strconv.Itoa(opts.Rotate))
	b.WriteString(";flip=")
	b.WriteString(opts.Flip)
	b.WriteString(";gray=")
	b.WriteString(strconv.FormatBool(opts.Grayscale))
	b.WriteString(";blur=")
	b.WriteString(strconv.FormatFloat(opts.Blur, 'g', -1, 64))
	b.WriteString(";sharp=")
	b.WriteString(strconv.FormatFloat(opts.Sharpen, 'g', -1, 64))
	if opts.Watermark != nil {
		b.WriteString(";wm=")
		b.WriteString(opts.Watermark.Text)
		b.WriteString("|")
		b.WriteString(opts.Watermark.Position)
		b.WriteString("|")
		b.WriteString(strconv.Itoa(opts.Watermark.Opacity))
		b.WriteString("|")
		b.WriteString(strconv.Itoa(opts.Watermark.Size))
		b.WriteString("|")
		b.WriteString(opts.Watermark.Color)
	}
	return b.String()
}
