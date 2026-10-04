package imaging

import (
	"bytes"
	"image"
	"image/png"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// decodeImage 将字节解码为 image.Image，返回其像素。
func decodeImage(src []byte) (image.Image, string, error) {
	return image.Decode(bytes.NewReader(src))
}

// encodePNG 将图像编码为 PNG 字节。
func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
