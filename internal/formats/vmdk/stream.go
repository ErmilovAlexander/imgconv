package vmdk

import (
	"fmt"
	"io"
	"os"
)

type streamOptimizedExtent struct {
	f *os.File
	h *sparseHeader

	gm *streamGrainMap

	sizeBytes    uint64
	grainBytes   uint64
	grainSectors uint64
}

func openStreamOptimizedExtent(path string) (*streamOptimizedExtent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	h, err := readSparseHeader(f)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if h.Layout() != layoutStreamOptimized {
		_ = f.Close()
		return nil, fmt.Errorf("%w: not streamOptimized", ErrUnsupportedVMDK)
	}

	dbg("streamOptimized header: capacitySectors=%d grainSectors=%d numGTEsPerGT=%d descOff=%d descSize=%d overHead=%d gdOff=%d rgdOff=%d",
		h.CapacitySectors, h.GrainSizeSectors, h.NumGTEsPerGT, h.DescriptorOffset, h.DescriptorSize, h.OverHeadSectors, h.GdOffset, h.RgdOffset)

	events, err := scanStreamOptimized(f, h)
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	if debugOn() {
		types := countEventTypes(events)
		dbg("events: total=%d EOS=%d GD=%d GT=%d FOOTER=%d DATA=%d",
			len(events),
			types[streamMarkerEOS],
			types[streamMarkerGD],
			types[streamMarkerGT],
			types[streamMarkerFOOTER],
			types[streamEventDATA],
		)
	}

	gm, err := buildStreamGrainMap(events, h)
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	grainBytes := uint64(h.GrainSizeSectors) * sectorSize
	sizeBytes := uint64(h.CapacitySectors) * sectorSize

	if debugOn() {
		dbg("grainMap: grains=%d", len(gm.grains))
		if p0, ok := gm.get(0); ok {
			dbg("grain0 payload: markerOff=%d payloadOff=%d sizeBytes=%d dupCount=%d lba=%d",
				p0.markerOff, p0.offBytes, p0.sizeBytes, p0.dupCount, p0.lbaSectors)
			g0, err := readPayload(f, p0, grainBytes)
			if err != nil {
				dbg("grain0 read/decompress error: %v", err)
			} else {
				dbg("grain0 first 64 bytes:\n%s", hexdumpPrefix(g0, 64))
			}
		} else {
			dbg("grain0: not present (zeros)")
		}

		// 🎯 Focused grain probe
		if g, ok := debugGrainTarget(); ok {
			dumpStreamGrain(f, gm, grainBytes, uint64(h.GrainSizeSectors), g)
		}
	}

	return &streamOptimizedExtent{
		f:            f,
		h:            h,
		gm:           gm,
		sizeBytes:    sizeBytes,
		grainBytes:   grainBytes,
		grainSectors: uint64(h.GrainSizeSectors),
	}, nil
}

func dumpStreamGrain(f *os.File, gm *streamGrainMap, grainBytes uint64, grainSectors uint64, grainIdx uint64) {
	dbg("---- DEBUG GRAIN %d ----", grainIdx)
	lba := grainIdx * grainSectors
	dbg("grain=%d -> LBA=%d sectors, grainBytes=%d", grainIdx, lba, grainBytes)

	pd, ok := gm.get(grainIdx)
	if !ok {
		dbg("grain %d not present in map => ZERO", grainIdx)
		dbg("---- END DEBUG GRAIN %d ----", grainIdx)
		return
	}

	pfx, err := payloadPrefix4(f, pd.offBytes)
	if err != nil {
		dbg("grain %d payloadPrefix read error: %v", grainIdx, err)
	} else {
		dbg("grain %d payload prefix u32le=0x%08x (bytes=%d)", grainIdx, pfx, pd.sizeBytes)
	}

	dbg("grain %d mapping: markerOff=%d payloadOff=%d sizeBytes=%d dupCount=%d lba=%d",
		grainIdx, pd.markerOff, pd.offBytes, pd.sizeBytes, pd.dupCount, pd.lbaSectors)

	data, err := readPayload(f, pd, grainBytes)
	if err != nil {
		dbg("grain %d readPayload error: %v", grainIdx, err)
		dbg("---- END DEBUG GRAIN %d ----", grainIdx)
		return
	}

	dbg("grain %d decompressed CRC32=0x%08x", grainIdx, crc32Of(data))
	dbg("grain %d first 64 bytes:\n%s", grainIdx, hexdumpPrefix(data, 64))
	dbg("grain %d last 64 bytes:\n%s", grainIdx, hexdumpPrefix(data[len(data)-64:], 64))
	dbg("---- END DEBUG GRAIN %d ----", grainIdx)
}

func (e *streamOptimizedExtent) Close() error { return e.f.Close() }
func (e *streamOptimizedExtent) Size() uint64 { return e.sizeBytes }

func (e *streamOptimizedExtent) ReadAt(p []byte, off int64) (int, error) {
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
		curOff := uint64(off) + uint64(read)
		grainIdx := curOff / e.grainBytes
		inGrain := curOff % e.grainBytes
		want := minU64(uint64(len(p)-read), e.grainBytes-inGrain)

		pd, ok := e.gm.get(grainIdx)
		if !ok {
			zeroFill(p[read : read+int(want)])
			read += int(want)
			continue
		}

		grainData, err := readPayload(e.f, pd, e.grainBytes)
		if err != nil {
			return read, err
		}
		copy(p[read:read+int(want)], grainData[inGrain:inGrain+want])
		read += int(want)
	}

	if uint64(off)+uint64(read) >= e.sizeBytes {
		return read, io.EOF
	}
	return read, nil
}
