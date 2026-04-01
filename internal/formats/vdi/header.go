package vdi

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	ImageTypeDynamic = 1
	ImageTypeFixed   = 2

	ImageFlagsZeroExpand = 0x00020000

	BlockFree  = 0xFFFFFFFF
	BlockZero  = 0xFFFFFFFE
	SectorSize = 512

	vdiSignature = 0xBEDA107F
)

type Header struct {
	Text             [64]byte
	Signature        uint32
	Version          uint32
	HeaderSize       uint32
	ImageType        uint32
	ImageFlags       uint32
	Description      [256]byte
	BlocksOffset     uint32 // offset of block map
	DataOffset       uint32 // offset of image data
	Cylinders        uint32
	Heads            uint32
	Sectors          uint32
	SectorSize       uint32
	Reserved1        uint32 // IMPORTANT: exists for v1+ headers
	DiskSize         uint64
	BlockSize        uint32
	BlockExtra       uint32
	BlocksInHDD      uint32
	BlocksAllocated  uint32
	UUIDCreate       [16]byte
	UUIDModify       [16]byte
	UUIDLink         [16]byte
	UUIDParentModify [16]byte
	LCHSGeometryPad  [56]byte
}

func ReadHeader(r io.ReaderAt) (Header, error) {
	var raw [512]byte
	if _, err := r.ReadAt(raw[:], 0); err != nil {
		return Header{}, err
	}

	var h Header
	copy(h.Text[:], raw[0:64])

	h.Signature = binary.LittleEndian.Uint32(raw[64:68])
	if h.Signature != vdiSignature {
		return Header{}, fmt.Errorf("vdi: bad signature 0x%x", h.Signature)
	}

	h.Version = binary.LittleEndian.Uint32(raw[68:72])
	h.HeaderSize = binary.LittleEndian.Uint32(raw[72:76])

	h.ImageType = binary.LittleEndian.Uint32(raw[76:80])
	h.ImageFlags = binary.LittleEndian.Uint32(raw[80:84])
	copy(h.Description[:], raw[84:340])

	h.BlocksOffset = binary.LittleEndian.Uint32(raw[340:344])
	h.DataOffset = binary.LittleEndian.Uint32(raw[344:348])

	h.Cylinders = binary.LittleEndian.Uint32(raw[348:352])
	h.Heads = binary.LittleEndian.Uint32(raw[352:356])
	h.Sectors = binary.LittleEndian.Uint32(raw[356:360])
	h.SectorSize = binary.LittleEndian.Uint32(raw[360:364])

	// IMPORTANT:
	// for v1+ headers there is a reserved u32 here before disk_size.
	h.Reserved1 = binary.LittleEndian.Uint32(raw[364:368])

	// DiskSize starts at 368, not 364.
	h.DiskSize = binary.LittleEndian.Uint64(raw[368:376])

	h.BlockSize = binary.LittleEndian.Uint32(raw[376:380])
	h.BlockExtra = binary.LittleEndian.Uint32(raw[380:384])
	h.BlocksInHDD = binary.LittleEndian.Uint32(raw[384:388])
	h.BlocksAllocated = binary.LittleEndian.Uint32(raw[388:392])

	copy(h.UUIDCreate[:], raw[392:408])
	copy(h.UUIDModify[:], raw[408:424])
	copy(h.UUIDLink[:], raw[424:440])
	copy(h.UUIDParentModify[:], raw[440:456])
	copy(h.LCHSGeometryPad[:], raw[456:512])

	if h.SectorSize == 0 {
		h.SectorSize = SectorSize
	}
	if h.BlockSize == 0 {
		return Header{}, fmt.Errorf("vdi: zero block size")
	}
	if h.DiskSize == 0 {
		return Header{}, fmt.Errorf("vdi: zero disk size")
	}
	if h.BlocksInHDD == 0 {
		return Header{}, fmt.Errorf("vdi: zero blocks in hdd")
	}
	if h.DataOffset == 0 {
		return Header{}, fmt.Errorf("vdi: zero data offset")
	}

	return h, nil
}

func (h Header) EntriesOffset() uint64 {
	return uint64(h.BlocksOffset)
}

func (h Header) EntriesSize() uint64 {
	return uint64(h.BlocksInHDD) * 4
}

func (h Header) DataStart() uint64 {
	return uint64(h.DataOffset)
}

func (h Header) BlockDataOffset(blockIndex uint32) uint64 {
	return h.DataStart() + uint64(blockIndex)*uint64(h.BlockSize+h.BlockExtra)
}
