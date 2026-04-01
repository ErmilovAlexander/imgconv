package vmdk

import (
	"fmt"
	"io"
	"os"
)

type flatExtent struct {
	f          *os.File
	sizeBytes  uint64
	dataOffset uint64 // bytes
}

func openFlatExtent(path string, sectors uint64, flatOffsetSectors uint64) (*flatExtent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &flatExtent{
		f:          f,
		sizeBytes:  sectors * sectorSize,
		dataOffset: flatOffsetSectors * sectorSize,
	}, nil
}

func (e *flatExtent) Close() error { return e.f.Close() }
func (e *flatExtent) Size() uint64 { return e.sizeBytes }

func (e *flatExtent) ReadAt(p []byte, off int64) (int, error) {
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

	fileOff := int64(e.dataOffset) + off
	n, err := e.f.ReadAt(p, fileOff)
	if err != nil && err != io.EOF {
		return n, fmt.Errorf("flat readat: %w", err)
	}
	if uint64(off)+uint64(n) >= e.sizeBytes {
		return n, io.EOF
	}
	return n, nil
}
