package vmdk

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

const invalidSector = ^uint64(0)

// embeddedSparseExtent reads sparse data embedded inside streamOptimized container.
type embeddedSparseExtent struct {
	f *os.File
	h *sparseHeader

	gd []uint64
	gt map[uint64][]uint64

	numGTEs    uint64
	grainBytes uint64
	sizeBytes  uint64
}

func openEmbeddedSparseExtent(path string) (extent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	h, err := readSparseHeader(f)
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	if debugOn() {
		dbg("embeddedSparse open: layout=%d cap=%d grain=%d gtes=%d descOff=%d descSize=%d overHead=%d gd=%d rgd=%d",
			h.Layout(), h.CapacitySectors, h.GrainSizeSectors, h.NumGTEsPerGT, h.DescriptorOffset, h.DescriptorSize, h.OverHeadSectors, h.GdOffset, h.RgdOffset)
	}

	gdSector := pickGDSector(h)
	if gdSector == invalidSector {
		dbg("embeddedSparse: primary gd=%d redundant gd=%d are invalid; probing footer...", h.GdOffset, h.RgdOffset)

		// Try footer header
		fh, fhoff, ferr := tryReadFooterHeader(path)
		if ferr != nil {
			_ = f.Close()
			return nil, fmt.Errorf("%w: no valid GD/RGD offset (and footer probe failed: %v)", ErrUnsupportedVMDK, ferr)
		}

		dbg("embeddedSparse: using footer header at off=%d (sector=%d)", fhoff, fhoff/int64(sectorSize))
		// prefer footer GD/RGD
		gdSector = pickGDSector(fh)
		if gdSector == invalidSector {
			_ = f.Close()
			return nil, fmt.Errorf("%w: footer header also has no valid GD/RGD offset", ErrUnsupportedVMDK)
		}

		// Note: we still read data from original file, but use footer header fields for GD offsets.
		// Replace h with footer header for addressing.
		h = fh
	}

	if h.NumGTEsPerGT == 0 || h.GrainSizeSectors == 0 {
		_ = f.Close()
		return nil, fmt.Errorf("%w: invalid sparse header fields (NumGTEsPerGT/GrainSizeSectors)", ErrUnsupportedVMDK)
	}

	gd, gt, err := loadEmbeddedGDGT(f, h, gdSector)
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	grainBytes := uint64(h.GrainSizeSectors) * sectorSize
	sizeBytes := uint64(h.CapacitySectors) * sectorSize
	numGTEs := uint64(h.NumGTEsPerGT)

	dbg("embeddedSparse: gdSector=%d gdEntries=%d gtTables=%d grainBytes=%d sizeBytes=%d",
		gdSector, len(gd), len(gt), grainBytes, sizeBytes)

	return &embeddedSparseExtent{
		f:          f,
		h:          h,
		gd:         gd,
		gt:         gt,
		numGTEs:    numGTEs,
		grainBytes: grainBytes,
		sizeBytes:  sizeBytes,
	}, nil
}

func pickGDSector(h *sparseHeader) uint64 {
	if h.GdOffset != 0 && h.GdOffset != invalidSector {
		dbg("embeddedSparse: using primary GD offset=%d", h.GdOffset)
		return h.GdOffset
	}
	if h.RgdOffset != 0 && h.RgdOffset != invalidSector {
		dbg("embeddedSparse: using redundant GD offset=%d", h.RgdOffset)
		return h.RgdOffset
	}
	return invalidSector
}

func loadEmbeddedGDGT(
	f *os.File,
	h *sparseHeader,
	gdSector uint64,
) ([]uint64, map[uint64][]uint64, error) {

	grainSectors := uint64(h.GrainSizeSectors)
	numGTEs := uint64(h.NumGTEsPerGT)

	numGrains := (uint64(h.CapacitySectors) + grainSectors - 1) / grainSectors
	numGTs := (numGrains + numGTEs - 1) / numGTEs

	dbg("embeddedSparse: computed numGrains=%d numGTs=%d (cap=%d grainSectors=%d numGTEs=%d)",
		numGrains, numGTs, h.CapacitySectors, h.GrainSizeSectors, h.NumGTEsPerGT)

	// --- Read GD (uint32 entries) ---
	gd := make([]uint64, numGTs)

	gdBytes := numGTs * 4
	gdSectors := (gdBytes + sectorSize - 1) / sectorSize
	gdBuf := make([]byte, gdSectors*sectorSize)

	dbg("embeddedSparse: read GD at sector=%d bytes=%d sectors=%d", gdSector, gdBytes, gdSectors)

	if _, err := f.ReadAt(gdBuf, int64(gdSector*sectorSize)); err != nil {
		return nil, nil, fmt.Errorf("read embedded GD at sector=%d: %w", gdSector, err)
	}

	for i := uint64(0); i < numGTs; i++ {
		off := binary.LittleEndian.Uint32(gdBuf[i*4 : i*4+4])
		gd[i] = uint64(off)
	}

	// quick stats
	if debugOn() {
		nz := 0
		for _, v := range gd {
			if v != 0 {
				nz++
			}
		}
		dbg("embeddedSparse: GD nonzero=%d/%d", nz, len(gd))
		for i := 0; i < 8 && i < len(gd); i++ {
			dbg("embeddedSparse: gd[%d]=%d", i, gd[i])
		}
	}

	// --- Read GTs ---
	gt := make(map[uint64][]uint64, numGTs)

	gtEntryBytes := numGTEs * 4
	gtSectors := (gtEntryBytes + sectorSize - 1) / sectorSize
	gtBuf := make([]byte, gtSectors*sectorSize)

	dbg("embeddedSparse: GT entryBytes=%d gtSectors=%d", gtEntryBytes, gtSectors)

	for gtIndex, gtSector := range gd {
		if gtSector == 0 {
			continue
		}

		if _, err := f.ReadAt(gtBuf, int64(gtSector*sectorSize)); err != nil {
			return nil, nil, fmt.Errorf("read embedded GT index=%d sector=%d: %w",
				gtIndex, gtSector, err)
		}

		table := make([]uint64, numGTEs)
		for i := uint64(0); i < numGTEs; i++ {
			off := binary.LittleEndian.Uint32(gtBuf[i*4 : i*4+4])
			table[i] = uint64(off)
		}

		gt[uint64(gtIndex)] = table
	}

	dbg("embeddedSparse: loaded GT tables=%d", len(gt))
	return gd, gt, nil
}

func (e *embeddedSparseExtent) Close() error { return e.f.Close() }
func (e *embeddedSparseExtent) Size() uint64 { return e.sizeBytes }

func (e *embeddedSparseExtent) ReadAt(p []byte, off int64) (int, error) {
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

		ps, ok := e.resolveGrain(grain)
		if !ok || ps == 0 {
			zeroFill(p[read : read+int(want)])
			read += int(want)
			continue
		}

		buf := make([]byte, e.grainBytes)
		if _, err := e.f.ReadAt(buf, int64(ps*sectorSize)); err != nil {
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

func (e *embeddedSparseExtent) resolveGrain(grain uint64) (uint64, bool) {
	if e.numGTEs == 0 {
		return 0, false
	}

	gtIndex := grain / e.numGTEs
	gtEntry := grain % e.numGTEs

	if gtIndex >= uint64(len(e.gd)) {
		return 0, false
	}

	gtSector := e.gd[gtIndex]
	if gtSector == 0 {
		return 0, true
	}

	table, ok := e.gt[gtIndex]
	if !ok || int(gtEntry) >= len(table) {
		return 0, true
	}

	return table[gtEntry], true
}
