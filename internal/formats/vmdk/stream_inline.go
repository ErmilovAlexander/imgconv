package vmdk

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// streamOptimizedInlineExtent implements streamOptimized VMDK with
// inline GD/GT and DENSE payload layout.
//
// RULES (verified vs qemu-img):
//   - GD entry != 0  => GT exists
//   - GT entries are NOT sparse indicators and NOT offsets
//   - All logical grains inside existing GT map to payload sequentially
//   - payloadGrain++ for EVERY logical grain in GT
type streamOptimizedInlineExtent struct {
	f *os.File
	h *sparseHeader

	payloadBaseSector uint64

	// logical grain -> payload grain index
	grainMap map[uint64]uint64

	grainSectors uint64
	grainBytes   uint64
	sizeBytes    uint64
}

func openStreamOptimizedInlineExtent(path string, h *sparseHeader) (extent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	grainSectors := uint64(h.GrainSizeSectors)
	grainBytes := grainSectors * sectorSize
	sizeBytes := uint64(h.CapacitySectors) * sectorSize
	numGTEs := uint64(h.NumGTEsPerGT)

	numLogicalGrains :=
		(uint64(h.CapacitySectors) + grainSectors - 1) / grainSectors
	numGTs := (numLogicalGrains + numGTEs - 1) / numGTEs

	// ----- layout -----
	baseSector := uint64(h.OverHeadSectors)

	gdBytes := numGTs * 4
	gdSectors := (gdBytes + sectorSize - 1) / sectorSize
	gdSector := baseSector

	gtEntryBytes := numGTEs * 4
	gtSectorsPerGT := (gtEntryBytes + sectorSize - 1) / sectorSize
	gtSector := gdSector + gdSectors

	// ----- read GD -----
	gdBuf := make([]byte, gdSectors*sectorSize)
	if _, err := f.ReadAt(gdBuf, int64(gdSector*sectorSize)); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("read inline GD: %w", err)
	}

	gd := make([]bool, numGTs)
	nonZeroGD := 0
	for i := uint64(0); i < numGTs; i++ {
		if binary.LittleEndian.Uint32(gdBuf[i*4:i*4+4]) != 0 {
			gd[i] = true
			nonZeroGD++
		}
	}

	dbg("streamInline layout:")
	dbg("  baseSector=%d", baseSector)
	dbg("  gdSector=%d gdSectors=%d numGTs=%d nonZeroGD=%d",
		gdSector, gdSectors, numGTs, nonZeroGD)
	dbg("  gtSector=%d gtSectorsPerGT=%d",
		gtSector, gtSectorsPerGT)
	dbg("  grainSectors=%d grainBytes=%d logicalGrains=%d",
		grainSectors, grainBytes, numLogicalGrains)

	// ----- build dense payload mapping -----
	grainMap := make(map[uint64]uint64, numLogicalGrains)
	payloadGrain := uint64(0)
	curGTSector := gtSector

	for gtIndex := uint64(0); gtIndex < numGTs; gtIndex++ {
		if !gd[gtIndex] {
			continue
		}

		// GT exists → consume GT block (content ignored)
		curGTSector += gtSectorsPerGT

		for j := uint64(0); j < numGTEs; j++ {
			logicalGrain := gtIndex*numGTEs + j
			if logicalGrain >= numLogicalGrains {
				break
			}
			grainMap[logicalGrain] = payloadGrain
			payloadGrain++
		}
	}

	payloadBaseSector := curGTSector

	dbg("streamInline dense payload mapping:")
	dbg("  payloadGrains=%d payloadBaseSector=%d",
		payloadGrain, payloadBaseSector)

	if debugOn() {
		for i := uint64(0); i < 8; i++ {
			if pg, ok := grainMap[i]; ok {
				dbg("  grainMap[%d] -> payloadGrain %d", i, pg)
			} else {
				dbg("  grainMap[%d] -> ZERO", i)
			}
		}
	}

	return &streamOptimizedInlineExtent{
		f:                 f,
		h:                 h,
		payloadBaseSector: payloadBaseSector,
		grainMap:          grainMap,
		grainSectors:      grainSectors,
		grainBytes:        grainBytes,
		sizeBytes:         sizeBytes,
	}, nil
}

func (e *streamOptimizedInlineExtent) Close() error { return e.f.Close() }
func (e *streamOptimizedInlineExtent) Size() uint64 { return e.sizeBytes }

func (e *streamOptimizedInlineExtent) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, io.ErrUnexpectedEOF
	}
	if uint64(off) >= e.sizeBytes {
		return 0, io.EOF
	}

	max := e.sizeBytes - uint64(off)
	if uint64(len(p)) > max {
		p = p[:max]
	}

	read := 0
	for read < len(p) {
		cur := uint64(off) + uint64(read)
		grain := cur / e.grainBytes
		in := cur % e.grainBytes
		want := minU64(uint64(len(p)-read), e.grainBytes-in)

		pg, ok := e.grainMap[grain]
		if !ok {
			zeroFill(p[read : read+int(want)])
			read += int(want)
			continue
		}

		fileSector :=
			e.payloadBaseSector +
				pg*e.grainSectors

		if debugOn() && grain < 4 {
			dbg("READ grain=%d payloadGrain=%d fileSector=%d",
				grain, pg, fileSector)
		}

		buf := make([]byte, e.grainBytes)
		if _, err := e.f.ReadAt(buf, int64(fileSector*sectorSize)); err != nil {
			return read, err
		}

		copy(p[read:read+int(want)], buf[in:in+want])
		read += int(want)
	}

	if uint64(off)+uint64(read) >= e.sizeBytes {
		return read, io.EOF
	}
	return read, nil
}
