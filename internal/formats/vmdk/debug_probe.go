package vmdk

import (
	"encoding/hex"
	"fmt"
	"os"
)

func debugProbeVMDK(path string, h *sparseHeader) {
	if !debugOn() {
		return
	}

	f, err := os.Open(path)
	if err != nil {
		dbg("DEEP-PROBE: cannot open file: %v", err)
		return
	}
	defer f.Close()

	info, _ := f.Stat()
	fileSectors := info.Size() / sectorSize

	dbg("========== VMDK DEEP PROBE ==========")
	dbg("FILE")
	dbg("  path=%s", path)
	dbg("  sizeBytes=%d sizeSectors=%d", info.Size(), fileSectors)

	dbg("HEADER")
	dbg("  layout=%v", h.Layout())
	dbg("  capacitySectors=%d", h.CapacitySectors)
	dbg("  grainSizeSectors=%d", h.GrainSizeSectors)
	dbg("  numGTEsPerGT=%d", h.NumGTEsPerGT)
	dbg("  overHeadSectors=%d", h.OverHeadSectors)
	dbg("  descriptorOffset=%d", h.DescriptorOffset)
	dbg("  descriptorSize=%d", h.DescriptorSize)
	dbg("  gdOffset=%d", h.GdOffset)
	dbg("  rgdOffset=%d", h.RgdOffset)

	descEnd := uint64(h.DescriptorOffset + h.DescriptorSize)
	dbg("DERIVED")
	dbg("  descriptorEndSector=%d", descEnd)

	// Candidate regions
	candidates := []struct {
		name   string
		sector uint64
	}{
		{"sector0", 0},
		{"descriptor", uint64(h.DescriptorOffset)},
		{"descriptorEnd", descEnd},
		{"overHeadStart", uint64(h.OverHeadSectors)},
	}

	// try some nearby sectors too
	for _, off := range []int64{-2, -1, 1, 2, 8, 16, 32, 64} {
		s := int64(h.OverHeadSectors) + off
		if s > 0 {
			candidates = append(candidates, struct {
				name   string
				sector uint64
			}{
				fmt.Sprintf("overHead+%d", off),
				uint64(s),
			})
		}
	}

	dbg("SECTOR PROBES (first 64 bytes)")
	for _, c := range candidates {
		if c.sector >= uint64(fileSectors) {
			continue
		}
		buf := make([]byte, 64)
		_, err := f.ReadAt(buf, int64(c.sector*sectorSize))
		if err != nil {
			dbg("  %-16s sector=%-8d READ ERROR: %v", c.name, c.sector, err)
			continue
		}
		dbg("  %-16s sector=%-8d : %s",
			c.name,
			c.sector,
			hex.Dump(buf[:32]),
		)
	}

	dbg("========== END DEEP PROBE ==========")
}
