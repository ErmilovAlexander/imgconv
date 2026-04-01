package vmdk

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

const (
	streamMarkerEOS    = 0
	streamMarkerGT     = 1
	streamMarkerGD     = 2
	streamMarkerFOOTER = 3

	streamEventDATA = 100
)

type streamEvent struct {
	Type uint32

	// DATA marker:
	// val = LBA (sectors in virtual disk)
	// size = compressed payload size IN BYTES (for your image; validated by deep probe)
	LBA       uint64
	SizeBytes uint32

	// META marker:
	MetaSectors uint64

	PayloadOff int64 // DATA: markerOff+12, META: markerOff+512
	MarkerOff  int64 // absolute file offset of marker start (for debug)
}

func scanStreamOptimized(f *os.File, h *sparseHeader) ([]streamEvent, error) {
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	fileSize := st.Size()

	descEnd := int64((h.DescriptorOffset + h.DescriptorSize) * sectorSize)
	off := int64(h.OverHeadSectors * sectorSize)
	if off < descEnd {
		off = descEnd
	}
	if off < int64(sectorSize) {
		off = int64(sectorSize)
	}

	if debugOn() {
		dbg("scanStreamOptimized: fileSize=%d descEnd=%d ovh=%d startOff=%d",
			fileSize, descEnd, int64(h.OverHeadSectors*sectorSize), off)
	}

	var events []streamEvent
	markerIdx := 0

	for {
		if off < 0 || off+int64(sectorSize) > fileSize {
			return nil, fmt.Errorf("%w: marker off out of file: off=%d fileSize=%d", ErrUnsupportedVMDK, off, fileSize)
		}

		buf := make([]byte, sectorSize)
		n, err := f.ReadAt(buf, off)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("read marker sector at %d: %w", off, err)
		}
		if n != sectorSize {
			return nil, fmt.Errorf("read marker sector at %d: %w", off, ErrShortRead)
		}

		val := binary.LittleEndian.Uint64(buf[0:8])
		size := binary.LittleEndian.Uint32(buf[8:12])

		// DATA marker (compressed grain): size != 0, payload begins immediately at +12
		if size != 0 {
			ev := streamEvent{
				Type:       streamEventDATA,
				LBA:        val,
				SizeBytes:  size,
				PayloadOff: off + 12,
				MarkerOff:  off,
			}
			events = append(events, ev)

			// Advance: header(12) + payload(size bytes), padded to 512
			total := int64(12) + int64(size)
			step := alignUp(total, int64(sectorSize))
			next := off + step

			if debugOn() && markerIdx < 32 {
				pfx := buf[12:16]
				dbg("MARK[%d] DATA off=%d lba=%d sizeBytes=%d step=%d next=%d payloadPrefix=%02x %02x %02x %02x",
					markerIdx, off, val, size, step, next, pfx[0], pfx[1], pfx[2], pfx[3])
			}

			if next < off || next > fileSize {
				return nil, fmt.Errorf("%w: marker advance out of file: idx=%d off=%d sizeBytes=%d step=%d next=%d fileSize=%d",
					ErrUnsupportedVMDK, markerIdx, off, size, step, next, fileSize)
			}

			off = next
			markerIdx++
			continue
		}

		// META/EOS marker: size == 0, type present at +12
		typ := binary.LittleEndian.Uint32(buf[12:16])
		if typ != streamMarkerEOS && typ != streamMarkerGT && typ != streamMarkerGD && typ != streamMarkerFOOTER {
			return nil, fmt.Errorf("%w: unknown stream marker type %d (0x%x) at off=%d", ErrUnsupportedVMDK, typ, typ, off)
		}

		ev := streamEvent{
			Type:        typ,
			MetaSectors: val,
			PayloadOff:  off + int64(sectorSize),
			MarkerOff:   off,
		}
		events = append(events, ev)

		next := off + int64(sectorSize) + int64(val)*int64(sectorSize)

		if debugOn() && markerIdx < 32 {
			dbg("MARK[%d] META off=%d type=%s metaSectors=%d next=%d",
				markerIdx, off, markerName(typ), val, next)
		}

		if next < off || next > fileSize {
			return nil, fmt.Errorf("%w: meta marker advance out of file: idx=%d off=%d type=%d metaSectors=%d next=%d fileSize=%d",
				ErrUnsupportedVMDK, markerIdx, off, typ, val, next, fileSize)
		}

		off = next
		markerIdx++

		if typ == streamMarkerEOS {
			break
		}
	}

	return events, nil
}

func alignUp(v, a int64) int64 {
	if a <= 0 {
		return v
	}
	r := v % a
	if r == 0 {
		return v
	}
	return v + (a - r)
}
