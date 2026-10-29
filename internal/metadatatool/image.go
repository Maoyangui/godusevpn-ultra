package metadatatool

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	_ "image/jpeg"
	_ "image/png"
)

type imageInfo struct {
	format        string
	width, height int
}

func inspectImage(data []byte) (imageInfo, error) {
	if len(data) == 0 || len(data) > maxImage {
		return imageInfo{}, ErrInput
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 16384 || config.Height > 16384 || int64(config.Width)*int64(config.Height) > 16<<20 {
		return imageInfo{}, ErrInput
	}
	switch format {
	case "jpeg":
		if !singleJPEG(data) {
			return imageInfo{}, ErrInput
		}
		return imageInfo{"JPEG", config.Width, config.Height}, nil
	case "png":
		if !singlePNG(data) {
			return imageInfo{}, ErrInput
		}
		return imageInfo{"PNG", config.Width, config.Height}, nil
	}
	return imageInfo{}, ErrInput
}
func singlePNG(data []byte) bool {
	if len(data) < 8 || string(data[:8]) != "\x89PNG\r\n\x1a\n" {
		return false
	}
	for offset := 8; offset+12 <= len(data); {
		n := int(binary.BigEndian.Uint32(data[offset:]))
		if n > len(data)-offset-12 {
			return false
		}
		end := offset + n + 12
		tag := string(data[offset+4 : offset+8])
		if crc32.ChecksumIEEE(data[offset+4:end-4]) != binary.BigEndian.Uint32(data[end-4:end]) {
			return false
		}
		if tag == "acTL" || tag == "fcTL" || tag == "fdAT" {
			return false
		}
		if tag == "IEND" {
			return n == 0 && end == len(data)
		}
		offset = end
	}
	return false
}
func singleJPEG(data []byte) bool {
	if len(data) < 4 || data[0] != 0xff || data[1] != 0xd8 {
		return false
	}
	position := 2
	entropy := false
	for position < len(data) {
		if entropy {
			for position < len(data) && data[position] != 0xff {
				position++
			}
			if position+1 >= len(data) {
				return false
			}
			if data[position+1] == 0 || data[position+1] >= 0xd0 && data[position+1] <= 0xd7 {
				position += 2
				continue
			}
			entropy = false
		}
		if data[position] != 0xff {
			return false
		}
		for position < len(data) && data[position] == 0xff {
			position++
		}
		if position >= len(data) {
			return false
		}
		marker := data[position]
		position++
		if marker == 0xd9 {
			return position == len(data)
		}
		if marker == 0 || marker == 0xd8 || marker >= 0xd0 && marker <= 0xd7 || marker == 0x01 || position+2 > len(data) {
			return false
		}
		n := int(binary.BigEndian.Uint16(data[position:]))
		if n < 2 || n > len(data)-position {
			return false
		}
		if marker == 0xe2 && n >= 6 && string(data[position+2:position+6]) == "MPF\x00" {
			return false
		}
		position += n
		if marker == 0xda {
			entropy = true
		}
	}
	return false
}
func samePixels(a, b []byte) error {
	left, _, err := image.Decode(bytes.NewReader(a))
	if err != nil {
		return err
	}
	right, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return err
	}
	if left.Bounds() != right.Bounds() {
		return ErrVerification
	}
	for y := left.Bounds().Min.Y; y < left.Bounds().Max.Y; y++ {
		for x := left.Bounds().Min.X; x < left.Bounds().Max.X; x++ {
			lr, lg, lb, la := left.At(x, y).RGBA()
			rr, rg, rb, ra := right.At(x, y).RGBA()
			if lr != rr || lg != rg || lb != rb || la != ra {
				return ErrVerification
			}
		}
	}
	return nil
}
