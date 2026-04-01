package vmdk

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
)

func minU64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

func zeroFill(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// tryDecompressZlib пытается распаковать zlib в ровно expectedSize байт.
func tryDecompressZlib(payload []byte, expectedSize int) ([]byte, bool, error) {
	if len(payload) < 2 {
		return nil, false, nil
	}
	// типичные заголовки zlib: 0x78 0x01 / 0x78 0x9C / 0x78 0xDA
	if payload[0] != 0x78 {
		return nil, false, nil
	}

	r, err := zlib.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, false, nil
	}
	defer r.Close()

	out := make([]byte, expectedSize)
	n, err := io.ReadFull(r, out)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, true, err
	}
	if n != expectedSize {
		// иногда возможно неполное — считаем ошибкой формата
		return nil, true, fmt.Errorf("zlib: expected %d bytes, got %d", expectedSize, n)
	}
	return out, true, nil
}
