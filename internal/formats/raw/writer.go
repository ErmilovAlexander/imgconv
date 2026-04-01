package raw

import (
	"fmt"
	"os"
)

type Writer struct {
	f      *os.File
	size   uint64
	sparse bool
}

type Options struct {
	Sparse bool
}

func Create(path string, size uint64, opts Options) (*Writer, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return nil, err
	}

	if err := f.Truncate(int64(size)); err != nil {
		f.Close()
		return nil, err
	}

	return &Writer{
		f:      f,
		size:   size,
		sparse: opts.Sparse,
	}, nil
}

func (w *Writer) Size() uint64 { return w.size }
func (w *Writer) Close() error { return w.f.Close() }

// 🔹 RangeWriter API
func (w *Writer) WriteAt(p []byte, off int64) (int, error) {
	if off < 0 || uint64(off) > w.size {
		return 0, fmt.Errorf("raw: invalid offset")
	}
	end := uint64(off) + uint64(len(p))
	if end > w.size {
		return 0, fmt.Errorf("raw: write beyond end")
	}
	return w.f.WriteAt(p, off)
}
