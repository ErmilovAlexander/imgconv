package vmdk

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

type WriterOptions struct {
	Sparse            bool
	GrainSizeSectors  uint64
	DescriptorSectors uint64
	NumGTEsPerGT      uint32
}

type Writer struct {
	f *os.File

	path string
	size uint64

	h *sparseHeader

	sparse         bool
	grainBytes     uint64
	capacitySector uint64

	numGrains uint64
	numGTs    uint64

	gd       []uint32
	gtTables [][]uint32

	gdOffsetSectors   uint64
	gtStartSectors    uint64
	gtSectorsPerTable uint64
	nextDataSector    uint64

	closed bool
}

func Create(path string, size uint64, opts WriterOptions) (*Writer, error) {
	if size == 0 {
		return nil, fmt.Errorf("vmdk: zero size")
	}

	grainSizeSectors := opts.GrainSizeSectors
	if grainSizeSectors == 0 {
		grainSizeSectors = 128
	}
	if !isPowerOfTwo(grainSizeSectors) || grainSizeSectors <= 8 {
		return nil, fmt.Errorf("vmdk: grain size must be power-of-two and > 8 sectors")
	}

	descriptorSectors := opts.DescriptorSectors
	if descriptorSectors == 0 {
		descriptorSectors = 20
	}

	numGTEsPerGT := opts.NumGTEsPerGT
	if numGTEsPerGT == 0 {
		numGTEsPerGT = 512
	}
	if numGTEsPerGT != 512 {
		return nil, fmt.Errorf("vmdk: only NumGTEsPerGT=512 is supported in this writer")
	}

	capacitySectors := ceilDiv(size, sectorSize)
	numGrains := ceilDiv(capacitySectors, grainSizeSectors)
	numGTs := ceilDiv(numGrains, uint64(numGTEsPerGT))

	gtBytesPerTable := uint64(numGTEsPerGT) * 4
	gtSectorsPerTable := ceilDiv(gtBytesPerTable, sectorSize)

	gdBytes := numGTs * 4
	gdSectors := ceilDiv(gdBytes, sectorSize)

	gdOffsetSectors := uint64(1) + descriptorSectors
	gtStartSectors := gdOffsetSectors + gdSectors
	overHeadSectors := gtStartSectors + numGTs*gtSectorsPerTable

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, err
	}

	w := &Writer{
		f:                 f,
		path:              path,
		size:              size,
		sparse:            opts.Sparse,
		grainBytes:        grainSizeSectors * sectorSize,
		capacitySector:    capacitySectors,
		numGrains:         numGrains,
		numGTs:            numGTs,
		gd:                make([]uint32, numGTs),
		gtTables:          make([][]uint32, numGTs),
		gdOffsetSectors:   gdOffsetSectors,
		gtStartSectors:    gtStartSectors,
		gtSectorsPerTable: gtSectorsPerTable,
		nextDataSector:    overHeadSectors,
		h: &sparseHeader{
			MagicNumber:      vmdkSparseMagicLE,
			Version:          1,
			Flags:            0x00000001,
			CapacitySectors:  capacitySectors,
			GrainSizeSectors: grainSizeSectors,
			DescriptorOffset: 1,
			DescriptorSize:   descriptorSectors,
			NumGTEsPerGT:     numGTEsPerGT,
			RgdOffset:        0,
			GdOffset:         gdOffsetSectors,
			OverHeadSectors:  overHeadSectors,
		},
	}

	for i := uint64(0); i < numGTs; i++ {
		w.gd[i] = uint32(gtStartSectors + i*gtSectorsPerTable)
		w.gtTables[i] = make([]uint32, numGTEsPerGT)
	}

	if err := f.Truncate(int64(overHeadSectors * sectorSize)); err != nil {
		_ = f.Close()
		return nil, err
	}

	if err := w.writeHeader(); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := w.writeDescriptor(); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := w.writeGD(); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := w.writeAllGTs(); err != nil {
		_ = f.Close()
		return nil, err
	}

	return w, nil
}

func (w *Writer) Size() uint64 { return w.size }

func (w *Writer) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true

	if err := w.writeHeader(); err != nil {
		_ = w.f.Close()
		return err
	}
	if err := w.writeDescriptor(); err != nil {
		_ = w.f.Close()
		return err
	}
	if err := w.writeGD(); err != nil {
		_ = w.f.Close()
		return err
	}
	if err := w.writeAllGTs(); err != nil {
		_ = w.f.Close()
		return err
	}
	return w.f.Close()
}

func (w *Writer) WriteAt(p []byte, off int64) (int, error) {
	if w.closed {
		return 0, fmt.Errorf("vmdk: write on closed writer")
	}
	if off < 0 {
		return 0, fmt.Errorf("vmdk: negative offset")
	}
	if uint64(off) >= w.size {
		return 0, nil
	}

	max := w.size - uint64(off)
	if uint64(len(p)) > max {
		p = p[:max]
	}

	written := 0
	for written < len(p) {
		curOff := uint64(off) + uint64(written)
		grainIdx := curOff / w.grainBytes
		inGrain := curOff % w.grainBytes

		want := uint64(len(p) - written)
		if want > w.grainBytes-inGrain {
			want = w.grainBytes - inGrain
		}

		chunk := p[written : written+int(want)]

		if w.sparse && inGrain == 0 && want == w.grainBytes && isAllZero(chunk) {
			written += int(want)
			continue
		}

		grainSector, err := w.ensureGrain(grainIdx)
		if err != nil {
			return written, err
		}

		fileOff := uint64(grainSector)*sectorSize + inGrain
		n, err := w.f.WriteAt(chunk, int64(fileOff))
		written += n
		if err != nil {
			return written, err
		}
	}

	return written, nil
}

func (w *Writer) ensureGrain(grainIdx uint64) (uint32, error) {
	gtIdx := grainIdx / uint64(w.h.NumGTEsPerGT)
	gtEnt := grainIdx % uint64(w.h.NumGTEsPerGT)

	if gtIdx >= uint64(len(w.gtTables)) {
		return 0, fmt.Errorf("vmdk: grain index out of range")
	}

	if sec := w.gtTables[gtIdx][gtEnt]; sec != 0 {
		return sec, nil
	}

	if w.nextDataSector > math.MaxUint32 {
		return 0, fmt.Errorf("vmdk: sector offset exceeds uint32")
	}

	sec := uint32(w.nextDataSector)
	w.gtTables[gtIdx][gtEnt] = sec
	w.nextDataSector += w.h.GrainSizeSectors

	if err := w.f.Truncate(int64(w.nextDataSector * sectorSize)); err != nil {
		return 0, err
	}

	if err := w.writeGT(gtIdx); err != nil {
		return 0, err
	}
	return sec, nil
}

func (w *Writer) writeHeader() error {
	buf := make([]byte, 512)

	binary.LittleEndian.PutUint32(buf[0:4], w.h.MagicNumber)
	binary.LittleEndian.PutUint32(buf[4:8], w.h.Version)
	binary.LittleEndian.PutUint32(buf[8:12], w.h.Flags)
	binary.LittleEndian.PutUint64(buf[12:20], w.h.CapacitySectors)
	binary.LittleEndian.PutUint64(buf[20:28], w.h.GrainSizeSectors)
	binary.LittleEndian.PutUint64(buf[28:36], w.h.DescriptorOffset)
	binary.LittleEndian.PutUint64(buf[36:44], w.h.DescriptorSize)
	binary.LittleEndian.PutUint32(buf[44:48], w.h.NumGTEsPerGT)
	binary.LittleEndian.PutUint64(buf[48:56], w.h.RgdOffset)
	binary.LittleEndian.PutUint64(buf[56:64], w.h.GdOffset)
	binary.LittleEndian.PutUint64(buf[64:72], w.h.OverHeadSectors)

	buf[72] = 0
	buf[73] = '\n'
	buf[74] = ' '
	buf[75] = '\r'
	buf[76] = '\n'
	binary.LittleEndian.PutUint16(buf[77:79], 0)

	_, err := w.f.WriteAt(buf, 0)
	return err
}

func (w *Writer) writeDescriptor() error {
	desc := buildEmbeddedDescriptor(filepath.Base(w.path), w.h.CapacitySectors)
	maxBytes := w.h.DescriptorSize * sectorSize
	if uint64(len(desc)) > maxBytes {
		return fmt.Errorf("vmdk: descriptor too large for embedded area")
	}

	buf := make([]byte, maxBytes)
	copy(buf, desc)

	off, err := sectorsToOffBytes(w.h.DescriptorOffset)
	if err != nil {
		return err
	}
	_, err = w.f.WriteAt(buf, off)
	return err
}

func (w *Writer) writeGD() error {
	buf := make([]byte, ceilDiv(uint64(len(w.gd))*4, sectorSize)*sectorSize)
	for i, v := range w.gd {
		binary.LittleEndian.PutUint32(buf[i*4:i*4+4], v)
	}

	off, err := sectorsToOffBytes(w.gdOffsetSectors)
	if err != nil {
		return err
	}
	_, err = w.f.WriteAt(buf, off)
	return err
}

func (w *Writer) writeAllGTs() error {
	for i := uint64(0); i < uint64(len(w.gtTables)); i++ {
		if err := w.writeGT(i); err != nil {
			return err
		}
	}
	return nil
}

func (w *Writer) writeGT(gtIdx uint64) error {
	if gtIdx >= uint64(len(w.gtTables)) {
		return fmt.Errorf("vmdk: gt index out of range")
	}
	buf := make([]byte, w.gtSectorsPerTable*sectorSize)
	for i, v := range w.gtTables[gtIdx] {
		binary.LittleEndian.PutUint32(buf[i*4:i*4+4], v)
	}

	off, err := sectorsToOffBytes(uint64(w.gd[gtIdx]))
	if err != nil {
		return err
	}
	_, err = w.f.WriteAt(buf, off)
	return err
}

func buildEmbeddedDescriptor(baseName string, capacitySectors uint64) []byte {
	baseName = strings.ReplaceAll(baseName, `"`, `_`)
	heads := 16
	sectors := 63
	cylinders := int(capacitySectors / uint64(heads*sectors))
	if cylinders <= 0 {
		cylinders = 1
	}
	if cylinders > 16383 {
		cylinders = 16383
	}

	desc := fmt.Sprintf(`# Disk DescriptorFile
version=1
CID=fffffffe
parentCID=ffffffff
createType="monolithicSparse"

# Extent description
RW %d SPARSE "%s"

# The Disk Data Base
#DDB
ddb.virtualHWVersion = "4"
ddb.geometry.cylinders = "%d"
ddb.geometry.heads = "%d"
ddb.geometry.sectors = "%d"
ddb.adapterType = "lsilogic"
ddb.toolsVersion = "0"
ddb.thinProvisioned = "1"
`, capacitySectors, baseName, cylinders, heads, sectors)

	return []byte(desc)
}

func ceilDiv(x, y uint64) uint64 {
	if y == 0 {
		return 0
	}
	return (x + y - 1) / y
}

func isPowerOfTwo(v uint64) bool {
	return v != 0 && (v&(v-1)) == 0
}

func isAllZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}
