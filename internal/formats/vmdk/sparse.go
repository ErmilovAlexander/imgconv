package vmdk

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sync"
)

type hostedSparseExtent struct {
	f *os.File
	h *sparseHeader

	sizeBytes  uint64
	grainBytes uint64

	numGTEsPerGT uint32
	numGTs       uint64

	gdMu sync.Mutex
	gd   []uint32 // sector offsets of GT (32-bit entries in hosted sparse VMDK)

	gtMu    sync.Mutex
	gtCache map[uint64][]uint32 // gtIdx -> grain sector offsets (32-bit entries)
}

func openHostedSparseExtent(path string) (*hostedSparseExtent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	h, err := readSparseHeader(f)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if h.Layout() != layoutHostedSparse {
		_ = f.Close()
		return nil, fmt.Errorf("%w: not hosted sparse", ErrUnsupportedVMDK)
	}

	grainBytes := h.GrainSizeSectors * sectorSize
	sizeBytes := h.CapacitySectors * sectorSize

	numGrains := (h.CapacitySectors + h.GrainSizeSectors - 1) / h.GrainSizeSectors
	numGTs := (numGrains + uint64(h.NumGTEsPerGT) - 1) / uint64(h.NumGTEsPerGT)

	e := &hostedSparseExtent{
		f:            f,
		h:            h,
		sizeBytes:    sizeBytes,
		grainBytes:   grainBytes,
		numGTEsPerGT: h.NumGTEsPerGT,
		numGTs:       numGTs,
		gd:           make([]uint32, numGTs),
		gtCache:      make(map[uint64][]uint32),
	}

	if err := e.loadGD(); err != nil {
		_ = f.Close()
		return nil, err
	}
	return e, nil
}

func (e *hostedSparseExtent) Close() error { return e.f.Close() }
func (e *hostedSparseExtent) Size() uint64 { return e.sizeBytes }

func (e *hostedSparseExtent) loadGD() error {
	e.gdMu.Lock()
	defer e.gdMu.Unlock()

	gdOffBytes, err := sectorsToOffBytes(e.h.GdOffset)
	if err != nil {
		return fmt.Errorf("GD offset: %w", err)
	}

	// Hosted sparse GD entries are uint32 sector offsets.
	buf := make([]byte, e.numGTs*4)
	_, err = e.f.ReadAt(buf, gdOffBytes)
	if err != nil && err != io.EOF {
		return fmt.Errorf("read GD: %w", err)
	}

	for i := uint64(0); i < e.numGTs; i++ {
		base := i * 4
		e.gd[i] = binary.LittleEndian.Uint32(buf[base : base+4])
	}
	return nil
}

func (e *hostedSparseExtent) getGT(gtIdx uint64) ([]uint32, error) {
	e.gtMu.Lock()
	defer e.gtMu.Unlock()

	if t, ok := e.gtCache[gtIdx]; ok {
		return t, nil
	}

	t := make([]uint32, e.numGTEsPerGT)

	if gtIdx >= uint64(len(e.gd)) {
		e.gtCache[gtIdx] = t
		return t, nil
	}

	gtSector := e.gd[gtIdx]
	if gtSector == 0 {
		e.gtCache[gtIdx] = t
		return t, nil
	}

	gtOff, err := sectorsToOffBytes(uint64(gtSector))
	if err != nil {
		return nil, fmt.Errorf("GT[%d] offset: %w", gtIdx, err)
	}

	// Hosted sparse GT entries are uint32 sector offsets.
	buf := make([]byte, uint64(e.numGTEsPerGT)*4)
	_, err = e.f.ReadAt(buf, gtOff)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("read GT[%d]: %w", gtIdx, err)
	}

	for i := uint64(0); i < uint64(e.numGTEsPerGT); i++ {
		base := i * 4
		t[i] = binary.LittleEndian.Uint32(buf[base : base+4])
	}

	e.gtCache[gtIdx] = t
	return t, nil
}

func (e *hostedSparseExtent) lookupGrain(grainIdx uint64) (bool, uint64, error) {
	gtIdx := grainIdx / uint64(e.numGTEsPerGT)
	gtEnt := grainIdx % uint64(e.numGTEsPerGT)

	gt, err := e.getGT(gtIdx)
	if err != nil {
		return false, 0, err
	}

	sec := gt[gtEnt]
	if sec == 0 {
		return false, 0, nil
	}

	return true, uint64(sec) * sectorSize, nil
}

func (e *hostedSparseExtent) ReadAt(p []byte, off int64) (int, error) {
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
		grainIdx := cur / e.grainBytes
		inGrain := cur % e.grainBytes
		want := minU64(uint64(len(p)-read), e.grainBytes-inGrain)

		ok, grainOff, err := e.lookupGrain(grainIdx)
		if err != nil {
			return read, err
		}

		if !ok {
			zeroFill(p[read : read+int(want)])
		} else {
			fileOff := grainOff + inGrain
			if fileOff < grainOff {
				return read, fmt.Errorf("grain read: offset overflow grainOff=%d inGrain=%d", grainOff, inGrain)
			}

			n, err := e.f.ReadAt(p[read:read+int(want)], int64(fileOff))
			if err != nil && err != io.EOF {
				return read, fmt.Errorf("grain read: %w", err)
			}
			if n < int(want) {
				zeroFill(p[read+n : read+int(want)])
			}
		}

		read += int(want)
	}

	if uint64(off)+uint64(read) >= e.sizeBytes {
		return read, io.EOF
	}
	return read, nil
}
