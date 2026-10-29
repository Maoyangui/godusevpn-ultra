package privacydata

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
)

func processPNG(input io.Reader, out io.Writer, size int64) ([]Block, error) {
	r := io.LimitReader(input, size)
	signature := make([]byte, 8)
	if _, err := io.ReadFull(r, signature); err != nil || string(signature) != "\x89PNG\r\n\x1a\n" {
		return nil, errors.New("PNG 签名损坏")
	}
	if _, err := out.Write(signature); err != nil {
		return nil, errors.New("输出写入失败")
	}
	remaining := size - 8
	blocks := []Block{}
	first := true
	seenData := false
	dataEnded := false
	for chunks := 0; chunks < 100000; chunks++ {
		if remaining < 12 {
			return nil, errors.New("PNG 块截断或缺少 IEND")
		}
		header := make([]byte, 8)
		if _, err := io.ReadFull(r, header); err != nil {
			return nil, errors.New("PNG 块头截断")
		}
		length := int64(binary.BigEndian.Uint32(header[:4]))
		kind := string(header[4:])
		if length > remaining-12 || length > 0x7fffffff {
			return nil, errors.New("PNG 块长度不合法")
		}
		for _, b := range header[4:] {
			if !(b >= 'A' && b <= 'Z') && !(b >= 'a' && b <= 'z') {
				return nil, errors.New("PNG 块类型不合法")
			}
		}
		if header[6] >= 'a' && header[6] <= 'z' {
			return nil, errors.New("PNG 块类型保留位不合法")
		}
		if first {
			if kind != "IHDR" || length != 13 {
				return nil, errors.New("PNG 首块必须为 IHDR")
			}
			first = false
		} else if kind == "IHDR" {
			return nil, errors.New("PNG 重复 IHDR")
		}
		if kind == "IDAT" {
			if dataEnded {
				return nil, errors.New("PNG IDAT 块不连续")
			}
			seenData = true
		} else if seenData {
			dataEnded = true
		}
		remove := kind == "eXIf" || kind == "tEXt" || kind == "zTXt" || kind == "iTXt"
		if remove {
			if len(blocks) >= 4096 {
				return nil, errors.New("PNG 元数据块过多，拒绝处理")
			}
			blocks = append(blocks, Block{"PNG " + kind, length + 12})
		} else if _, err := out.Write(header); err != nil {
			return nil, errors.New("输出写入失败")
		}
		h := crc32.NewIEEE()
		_, _ = h.Write(header[4:])
		writer := io.Writer(h)
		if !remove {
			writer = io.MultiWriter(out, h)
		}
		if _, err := io.CopyN(writer, r, length); err != nil {
			return nil, errors.New("PNG 块数据截断或输出写入失败")
		}
		var crc [4]byte
		if _, err := io.ReadFull(r, crc[:]); err != nil {
			return nil, errors.New("PNG 校验值截断")
		}
		if binary.BigEndian.Uint32(crc[:]) != h.Sum32() {
			return nil, errors.New("PNG CRC 校验失败")
		}
		if !remove {
			if _, err := out.Write(crc[:]); err != nil {
				return nil, errors.New("输出写入失败")
			}
		}
		remaining -= length + 12
		if kind == "IEND" {
			if length != 0 || !seenData || remaining != 0 {
				return nil, errors.New("PNG 结束块不合法或存在附加数据")
			}
			return blocks, nil
		}
	}
	return nil, errors.New("PNG 块数量超出处理上限")
}
