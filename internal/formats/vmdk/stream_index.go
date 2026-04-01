package vmdk

import (
	"encoding/binary"
	"fmt"
	"os"
)

// streamIndex represents full virtual-to-physical mapping
// built from GD / GT metadata markers.
//
// NOTE: with the fixed streamOptimized reader we do NOT need this index
// for conversion (we map via DATA grain markers). This file is kept only
// for future use / reference and must compile.
type streamIndex struct {
	grainSectors uint64
	grainBytes   uint64
	numGrains    uint64
	numGTEs      uint64

	// Grain Directory:
	// gd[gtIndex] = GT payload sector offset (0 = absent)
	gd []uint64

	// Grain Tables:
	// gt[gtIndex][entry] = payload sector offset (0 = zero grain)
	gt map[uint64][]uint64
}

// buildStreamIndex parses marker stream events and builds GD/GT index.
// It expects metadata events with ev.Type == streamMarkerGD/streamMarkerGT
// and metadata size in ev.MetaSectors.
func buildStreamIndex(
	f *os.File,
	events []streamEvent,
	h *sparseHeader,
) (*streamIndex, error) {

	numGTEs := uint64(h.NumGTEsPerGT)
	if numGTEs == 0 {
		return nil, fmt.Errorf("invalid NumGTEsPerGT")
	}

	grainSectors := uint64(h.GrainSizeSectors)
	grainBytes := grainSectors * sectorSize
	numGrains := (uint64(h.CapacitySectors) + grainSectors - 1) / grainSectors

	// GD size = ceil(numGrains / numGTEs)
	numGTs := (numGrains + numGTEs - 1) / numGTEs

	idx := &streamIndex{
		grainSectors: grainSectors,
		grainBytes:   grainBytes,
		numGrains:    numGrains,
		numGTEs:      numGTEs,
		gd:           make([]uint64, numGTs),
		gt:           make(map[uint64][]uint64),
	}

	// Pass 1: read GD
	for _, ev := range events {
		if ev.Type != streamMarkerGD {
			continue
		}
		if err := idx.loadGD(f, ev); err != nil {
			return nil, err
		}
	}

	// Pass 2: read GTs
	for _, ev := range events {
		if ev.Type != streamMarkerGT {
			continue
		}
		if err := idx.loadGT(f, ev); err != nil {
			return nil, err
		}
	}

	return idx, nil
}

// loadGD reads Grain Directory payload.
func (idx *streamIndex) loadGD(f *os.File, ev streamEvent) error {
	if ev.MetaSectors == 0 {
		return nil
	}

	buf := make([]byte, int(ev.MetaSectors)*sectorSize)
	if _, err := f.ReadAt(buf, ev.PayloadOff); err != nil {
		return fmt.Errorf("read GD payload: %w", err)
	}

	// Some specs store GD/GT entries as 32-bit sector offsets.
	// Here we keep the legacy 64-bit decode only as "best effort".
	// If you decide to use this index later, we should re-check entry width.
	entries := len(buf) / 8
	for i := 0; i < entries && i < len(idx.gd); i++ {
		off := binary.LittleEndian.Uint64(buf[i*8:])
		idx.gd[i] = off
	}

	return nil
}

// loadGT reads one Grain Table payload.
func (idx *streamIndex) loadGT(f *os.File, ev streamEvent) error {
	if ev.MetaSectors == 0 {
		return nil
	}

	buf := make([]byte, int(ev.MetaSectors)*sectorSize)
	if _, err := f.ReadAt(buf, ev.PayloadOff); err != nil {
		return fmt.Errorf("read GT payload: %w", err)
	}

	entries := len(buf) / 8
	table := make([]uint64, entries)

	for i := 0; i < entries; i++ {
		table[i] = binary.LittleEndian.Uint64(buf[i*8:])
	}

	// Identify which GT this is by its sector offset.
	// PayloadOff is byte offset of metadata payload; sector is PayloadOff/512.
	gtSector := uint64(ev.PayloadOff / sectorSize)

	// Find matching GD entry
	var gtIndex uint64 = ^uint64(0)
	for i, off := range idx.gd {
		if off == gtSector {
			gtIndex = uint64(i)
			break
		}
	}

	if gtIndex == ^uint64(0) {
		// GT not referenced in GD — ignore per spec
		return nil
	}

	idx.gt[gtIndex] = table
	return nil
}

// resolveGrain maps virtual grain index -> payload sector offset.
func (idx *streamIndex) resolveGrain(grain uint64) (payloadSector uint64, ok bool) {
	if grain >= idx.numGrains {
		return 0, false
	}

	gtIndex := grain / idx.numGTEs
	gtEntry := grain % idx.numGTEs

	if gtIndex >= uint64(len(idx.gd)) {
		return 0, false
	}

	gtSector := idx.gd[gtIndex]
	if gtSector == 0 {
		// Entire GT is zero
		return 0, true
	}

	table, ok := idx.gt[gtIndex]
	if !ok || int(gtEntry) >= len(table) {
		return 0, true
	}

	payloadSector = table[gtEntry]
	return payloadSector, true
}
