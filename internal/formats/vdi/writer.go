package vdi

import (
	"encoding/binary"
	"fmt"
	"os"
)

type WriterOptions struct {
	BlockSize uint32
	Sparse    bool
}

type Writer struct {
	f              *os.File
	h              Header
	entries        []uint32
	nextBlock      uint32
	closed         bool
	sparse         bool
	allocatedCount uint32
}

func Create(path string, size uint64, opts WriterOptions) (*Writer, error) {
	if size == 0 {
		return nil, fmt.Errorf("vdi: zero size")
	}

	blockSize := opts.BlockSize
	if blockSize == 0 {
		blockSize = 1 << 20
	}
	if blockSize%4096 != 0 {
		return nil, fmt.Errorf("vdi: block size must be multiple of 4096")
	}

	blocksInHDD := uint32(size / uint64(blockSize))
	if size%uint64(blockSize) != 0 {
		blocksInHDD++
	}

	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}

	h := Header{
		Signature:    vdiSignature,
		Version:      0x00010001,
		HeaderSize:   0x190,
		ImageType:    ImageTypeDynamic,
		ImageFlags:   ImageFlagsZeroExpand,
		BlocksOffset: 512,
		SectorSize:   SectorSize,
		Reserved1:    0,
		DiskSize:     size,
		BlockSize:    blockSize,
		BlockExtra:   0,
		BlocksInHDD:  blocksInHDD,
	}

	copy(h.Text[:], []byte("<<< Oracle VM VirtualBox Disk Image >>>\n"))
	copy(h.Description[:], []byte("imgconv generated VDI"))

	entriesSize := uint64(blocksInHDD) * 4
	dataOffset := uint64(h.BlocksOffset) + entriesSize
	if rem := dataOffset % 4096; rem != 0 {
		dataOffset += 4096 - rem
	}
	h.DataOffset = uint32(dataOffset)

	w := &Writer{
		f:       f,
		h:       h,
		entries: make([]uint32, blocksInHDD),
		sparse:  opts.Sparse,
	}

	for i := range w.entries {
		w.entries[i] = BlockFree
	}

	if err := w.writeHeader(); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := w.writeEntries(); err != nil {
		_ = f.Close()
		return nil, err
	}

	return w, nil
}

func (w *Writer) Size() uint64 {
	return w.h.DiskSize
}

func (w *Writer) WriteAt(p []byte, off int64) (int, error) {
	if w.closed {
		return 0, fmt.Errorf("vdi: write on closed writer")
	}
	if off < 0 {
		return 0, fmt.Errorf("vdi: negative offset")
	}
	if uint64(off) >= w.h.DiskSize {
		return 0, nil
	}

	max := w.h.DiskSize - uint64(off)
	if uint64(len(p)) > max {
		p = p[:max]
	}

	written := 0
	blockSize := uint64(w.h.BlockSize)

	for written < len(p) {
		curOff := uint64(off) + uint64(written)
		blockNo := curOff / blockSize
		inBlock := curOff % blockSize

		want := uint64(len(p) - written)
		if want > blockSize-inBlock {
			want = blockSize - inBlock
		}

		chunk := p[written : written+int(want)]

		if w.entries[blockNo] == BlockFree {
			if w.sparse && isZero(chunk) && inBlock == 0 && want == blockSize {
				written += int(want)
				continue
			}
			if err := w.allocateBlock(uint32(blockNo)); err != nil {
				return written, err
			}
		}

		entry := w.entries[blockNo]
		dataOff := w.h.BlockDataOffset(entry) + inBlock
		n, err := w.f.WriteAt(chunk, int64(dataOff))
		written += n
		if err != nil {
			return written, err
		}
	}

	return written, nil
}

func (w *Writer) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true

	w.h.BlocksAllocated = w.allocatedCount

	if err := w.writeEntries(); err != nil {
		_ = w.f.Close()
		return err
	}
	if err := w.writeHeader(); err != nil {
		_ = w.f.Close()
		return err
	}

	return w.f.Close()
}

func (w *Writer) allocateBlock(logical uint32) error {
	entry := w.nextBlock
	w.nextBlock++
	w.entries[logical] = entry
	w.allocatedCount++

	blockEnd := int64(w.h.BlockDataOffset(entry) + uint64(w.h.BlockSize))
	if err := w.f.Truncate(blockEnd); err != nil {
		return err
	}
	return nil
}

func (w *Writer) writeHeader() error {
	var raw [512]byte

	copy(raw[0:64], w.h.Text[:])
	binary.LittleEndian.PutUint32(raw[64:68], w.h.Signature)
	binary.LittleEndian.PutUint32(raw[68:72], w.h.Version)
	binary.LittleEndian.PutUint32(raw[72:76], w.h.HeaderSize)
	binary.LittleEndian.PutUint32(raw[76:80], w.h.ImageType)
	binary.LittleEndian.PutUint32(raw[80:84], w.h.ImageFlags)
	copy(raw[84:340], w.h.Description[:])
	binary.LittleEndian.PutUint32(raw[340:344], w.h.BlocksOffset)
	binary.LittleEndian.PutUint32(raw[344:348], w.h.DataOffset)
	binary.LittleEndian.PutUint32(raw[348:352], w.h.Cylinders)
	binary.LittleEndian.PutUint32(raw[352:356], w.h.Heads)
	binary.LittleEndian.PutUint32(raw[356:360], w.h.Sectors)
	binary.LittleEndian.PutUint32(raw[360:364], w.h.SectorSize)
	binary.LittleEndian.PutUint32(raw[364:368], w.h.Reserved1)
	binary.LittleEndian.PutUint64(raw[368:376], w.h.DiskSize)
	binary.LittleEndian.PutUint32(raw[376:380], w.h.BlockSize)
	binary.LittleEndian.PutUint32(raw[380:384], w.h.BlockExtra)
	binary.LittleEndian.PutUint32(raw[384:388], w.h.BlocksInHDD)
	binary.LittleEndian.PutUint32(raw[388:392], w.h.BlocksAllocated)
	copy(raw[392:408], w.h.UUIDCreate[:])
	copy(raw[408:424], w.h.UUIDModify[:])
	copy(raw[424:440], w.h.UUIDLink[:])
	copy(raw[440:456], w.h.UUIDParentModify[:])
	copy(raw[456:512], w.h.LCHSGeometryPad[:])

	_, err := w.f.WriteAt(raw[:], 0)
	return err
}

func (w *Writer) writeEntries() error {
	buf := make([]byte, len(w.entries)*4)
	for i, e := range w.entries {
		binary.LittleEndian.PutUint32(buf[i*4:i*4+4], e)
	}
	_, err := w.f.WriteAt(buf, int64(w.h.EntriesOffset()))
	return err
}

func isZero(p []byte) bool {
	for _, b := range p {
		if b != 0 {
			return false
		}
	}
	return true
}
