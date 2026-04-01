package vmdk

import (
	"fmt"
	"os"
)

const (
	footerScanMax = 2 << 20 // 2 MiB
)

// tryReadFooterHeader tries to find a valid sparse header near end of file.
// Many VMDKs keep a backup header in the footer area.
func tryReadFooterHeader(path string) (*sparseHeader, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	size := st.Size()
	if size < int64(sectorSize) {
		return nil, 0, fmt.Errorf("file too small")
	}

	// scan last up to footerScanMax bytes aligned to sector size
	scan := int64(footerScanMax)
	if scan > size {
		scan = size
	}
	start := size - scan

	// Align start to sector
	start = (start / int64(sectorSize)) * int64(sectorSize)

	// We'll attempt to read header at each sector boundary.
	// Header is 512 bytes; readSparseHeader reads at current offset (0) in file handle,
	// so we need a helper that reads from specific offset.
	for off := start; off <= size-int64(sectorSize); off += int64(sectorSize) {
		h, err := readSparseHeaderAt(f, off)
		if err != nil {
			continue
		}
		// Heuristics: capacity/grainSize must be nonzero, layout must be sparse/stream
		if h.CapacitySectors == 0 || h.GrainSizeSectors == 0 || h.NumGTEsPerGT == 0 {
			continue
		}
		if h.Layout() != layoutHostedSparse && h.Layout() != layoutStreamOptimized {
			continue
		}
		dbg("footer header found at off=%d (sector=%d): layout=%d cap=%d grain=%d gtes=%d gd=%d rgd=%d overHead=%d",
			off, off/int64(sectorSize), h.Layout(), h.CapacitySectors, h.GrainSizeSectors, h.NumGTEsPerGT, h.GdOffset, h.RgdOffset, h.OverHeadSectors)
		return h, off, nil
	}

	return nil, 0, fmt.Errorf("no footer sparse header found in last %d bytes", scan)
}

// readSparseHeaderAt reads a sparse header from a specific file offset.
func readSparseHeaderAt(f *os.File, off int64) (*sparseHeader, error) {
	cur, err := f.Seek(0, 1)
	if err != nil {
		return nil, err
	}
	_, err = f.Seek(off, 0)
	if err != nil {
		return nil, err
	}
	h, err := readSparseHeader(f)
	_, _ = f.Seek(cur, 0)
	return h, err
}
