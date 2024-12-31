package encoder

import (
	"bytes"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"

	"github.com/kolesa-team/go-webp/encoder"
	"github.com/kolesa-team/go-webp/webp"
)

type Encoder struct {
}

func (e Encoder) Decode(r io.Reader) (image.Image, error) {
	img, _, err := image.Decode(r)
	if err != nil {
		return nil, err
	}

	return img, nil
}

func (e Encoder) Encode(img image.Image) (io.Reader, string, error) {
	buf := new(bytes.Buffer)
	options, err := encoder.NewLossyEncoderOptions(encoder.PresetDefault, 75)
	if err != nil {
		return nil, "", err
	}

	if err := webp.Encode(buf, img, options); err != nil {
		return nil, "", err
	}

	return buf, "image/webp", nil
}
