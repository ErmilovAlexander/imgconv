package vmdk

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	sectorSize = 512

	// По спецификации hosted sparse:
	// special value meaning "at end" / "unused" for some layouts (notably streamOptimized)
	u64MinusOne = ^uint64(0)
)

type sparseHeader struct {
	MagicNumber      uint32 // 'KDMV' = 0x564D444B little-endian
	Version          uint32
	Flags            uint32
	CapacitySectors  uint64
	GrainSizeSectors uint64

	DescriptorOffset uint64 // sectors
	DescriptorSize   uint64 // sectors

	NumGTEsPerGT uint32
	RgdOffset    uint64 // sectors
	GdOffset     uint64 // sectors

	OverHeadSectors uint64
}

// Минимальная классификация “по спецификации”, без гаданий
type vmdkLayout int

const (
	layoutHostedSparse vmdkLayout = iota
	layoutStreamOptimized
	layoutSESparse // placeholder; детект будет расширен, когда добавим reader
)

func (h *sparseHeader) Layout() vmdkLayout {
	// Stream optimized: GDOffset = -1 по spec (и обычно RGD тоже -1)
	if h.GdOffset == u64MinusOne {
		return layoutStreamOptimized
	}
	// SESparse: в hosted header может не определяться так просто;
	// в MVP оставляем placeholder. Реальный SESparse детект добавим вместе с sesparse reader.
	return layoutHostedSparse
}

func readSparseHeader(r io.ReaderAt) (*sparseHeader, error) {
	buf := make([]byte, 512)
	n, err := r.ReadAt(buf, 0)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if n < 512 {
		return nil, ErrShortRead
	}

	h := &sparseHeader{}
	h.MagicNumber = binary.LittleEndian.Uint32(buf[0:4])
	if h.MagicNumber != vmdkSparseMagicLE {
		return nil, fmt.Errorf("%w: sparse magic mismatch: 0x%x", ErrNotVMDK, h.MagicNumber)
	}

	h.Version = binary.LittleEndian.Uint32(buf[4:8])
	h.Flags = binary.LittleEndian.Uint32(buf[8:12])
	h.CapacitySectors = binary.LittleEndian.Uint64(buf[12:20])
	h.GrainSizeSectors = binary.LittleEndian.Uint64(buf[20:28])

	h.DescriptorOffset = binary.LittleEndian.Uint64(buf[28:36])
	h.DescriptorSize = binary.LittleEndian.Uint64(buf[36:44])

	h.NumGTEsPerGT = binary.LittleEndian.Uint32(buf[44:48])
	h.RgdOffset = binary.LittleEndian.Uint64(buf[48:56])
	h.GdOffset = binary.LittleEndian.Uint64(buf[56:64])

	h.OverHeadSectors = binary.LittleEndian.Uint64(buf[64:72])

	if h.GrainSizeSectors == 0 || h.NumGTEsPerGT == 0 || h.CapacitySectors == 0 {
		return nil, fmt.Errorf("%w: invalid sparse header fields", ErrUnsupportedVMDK)
	}

	return h, nil
}
