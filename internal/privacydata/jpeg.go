package privacydata

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
)

func processJPEG(input io.Reader, out io.Writer, size int64) ([]Block, error) {
	r := bufio.NewReader(io.LimitReader(input, size))
	blocks := []Block{}
	write := func(b []byte) error { _, err := out.Write(b); return err }
	var signature [2]byte
	if _, err := io.ReadFull(r, signature[:]); err != nil || signature != [2]byte{0xff, 0xd8} {
		return nil, errors.New("JPEG 签名损坏")
	}
	if err := write(signature[:]); err != nil {
		return nil, errors.New("输出写入失败")
	}
	inScan := false
	seenScan := false
	var marker byte
	var one [1]byte
	for segments := 0; segments < 100000; segments++ {
		prefix := []byte{0xff}
		if inScan {
			// Entropy data contains stuffed 0xff bytes and restart markers; preserve
			// it exactly, including progressive scan boundaries and marker padding.
			for {
				b, err := r.ReadByte()
				if err != nil {
					return nil, errors.New("JPEG 缺少结束标记")
				}
				if b != 0xff {
					one[0] = b
					if err = write(one[:]); err != nil {
						return nil, errors.New("输出写入失败")
					}
					continue
				}
				prefix = []byte{0xff}
				for {
					marker, err = r.ReadByte()
					if err != nil {
						return nil, errors.New("JPEG 扫描数据截断")
					}
					if marker != 0xff {
						break
					}
					prefix = append(prefix, 0xff)
					if len(prefix) > 65536 {
						return nil, errors.New("JPEG 标记填充过长")
					}
				}
				if marker == 0x00 || (marker >= 0xd0 && marker <= 0xd7) {
					prefix = append(prefix, marker)
					if err = write(prefix); err != nil {
						return nil, errors.New("输出写入失败")
					}
					continue
				}
				inScan = false
				break
			}
		} else {
			b, err := r.ReadByte()
			if err != nil || b != 0xff {
				return nil, errors.New("JPEG 标记损坏或截断")
			}
			for {
				marker, err = r.ReadByte()
				if err != nil {
					return nil, errors.New("JPEG 标记截断")
				}
				if marker != 0xff {
					break
				}
				prefix = append(prefix, 0xff)
				if len(prefix) > 65536 {
					return nil, errors.New("JPEG 标记填充过长")
				}
			}
		}
		if marker == 0xd9 {
			if !seenScan {
				return nil, errors.New("JPEG 没有图像扫描数据")
			}
			prefix = append(prefix, marker)
			if err := write(prefix); err != nil {
				return nil, errors.New("输出写入失败")
			}
			if _, err := r.ReadByte(); err != io.EOF {
				return nil, errors.New("JPEG 结束后存在附加数据，拒绝保留未知载荷")
			}
			return blocks, nil
		}
		if marker == 0x00 || marker == 0xd8 || (marker >= 0xd0 && marker <= 0xd7) {
			return nil, errors.New("JPEG 标记顺序不合法")
		}
		if marker == 0x01 {
			prefix = append(prefix, marker)
			if err := write(prefix); err != nil {
				return nil, errors.New("输出写入失败")
			}
			continue
		}
		var length [2]byte
		if _, err := io.ReadFull(r, length[:]); err != nil {
			return nil, errors.New("JPEG 块长度截断")
		}
		n := int(binary.BigEndian.Uint16(length[:]))
		if n < 2 {
			return nil, errors.New("JPEG 块长度不合法")
		}
		payload := make([]byte, n-2)
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, errors.New("JPEG 元数据或图像块截断")
		}
		kind := ""
		switch marker {
		case 0xe1:
			kind = "JPEG APP1 (Exif/XMP)"
		case 0xed:
			kind = "JPEG APP13 (IPTC/Photoshop)"
		case 0xfe:
			kind = "JPEG COM"
		}
		if kind != "" {
			if len(blocks) >= 4096 {
				return nil, errors.New("JPEG 元数据块过多，拒绝处理")
			}
			blocks = append(blocks, Block{kind, int64(len(prefix) + 1 + n)})
			continue
		}
		prefix = append(prefix, marker)
		prefix = append(prefix, length[:]...)
		if err := write(prefix); err != nil {
			return nil, errors.New("输出写入失败")
		}
		if err := write(payload); err != nil {
			return nil, errors.New("输出写入失败")
		}
		if marker == 0xda {
			inScan = true
			seenScan = true
		}
	}
	return nil, errors.New("JPEG 标记数量超出处理上限")
}
