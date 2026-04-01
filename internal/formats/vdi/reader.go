package vdi

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

type Reader struct {
	f       *os.File
	h       Header
	entries []uint32
}

func Open(path string) (*Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	h, err := ReadHeader(f)
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	entries := make([]uint32, h.BlocksInHDD)
	buf := make([]byte, len(entries)*4)
	if _, err := f.ReadAt(buf, int64(h.EntriesOffset())); err != nil && err != io.EOF {
		_ = f.Close()
		return nil, err
	}
	for i := range entries {
		entries[i] = binary.LittleEndian.Uint32(buf[i*4 : i*4+4])
	}

	return &Reader{
		f:       f,
		h:       h,
		entries: entries,
	}, nil
}

func (r *Reader) Close() error {
	return r.f.Close()
}

func (r *Reader) Size() uint64 {
	return r.h.DiskSize
}

func (r *Reader) BlockSize() uint32 {
	return r.h.BlockSize
}

func (r *Reader) ImageType() uint32 {
	return r.h.ImageType
}

func (r *Reader) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, fmt.Errorf("vdi: negative offset")
	}
	if uint64(off) >= r.h.DiskSize {
		return 0, io.EOF
	}

	max := r.h.DiskSize - uint64(off)
	if uint64(len(p)) > max {
		p = p[:max]
	}

	read := 0
	blockSize := uint64(r.h.BlockSize)

	for read < len(p) {
		curOff := uint64(off) + uint64(read)
		blockNo := curOff / blockSize
		inBlock := curOff % blockSize

		want := uint64(len(p) - read)
		if want > blockSize-inBlock {
			want = blockSize - inBlock
		}

		entry := r.entries[blockNo]
		switch entry {
		case BlockFree, BlockZero:
			for i := 0; i < int(want); i++ {
				p[read+i] = 0
			}
		default:
			dataOff := r.h.BlockDataOffset(entry) + inBlock
			n, err := r.f.ReadAt(p[read:read+int(want)], int64(dataOff))
			read += n
			if err != nil && err != io.EOF {
				return read, err
			}
			if n < int(want) {
				for i := n; i < int(want); i++ {
					p[read-int(want)+i] = 0
				}
			}
			continue
		}

		read += int(want)
	}

	if uint64(off)+uint64(read) >= r.h.DiskSize {
		return read, io.EOF
	}
	return read, nil
}
