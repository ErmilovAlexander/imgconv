package raw

import "os"

type Reader struct {
	f    *os.File
	size uint64
}

func Open(path string) (*Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	return &Reader{
		f:    f,
		size: uint64(st.Size()),
	}, nil
}

func (r *Reader) ReadAt(p []byte, off int64) (int, error) {
	return r.f.ReadAt(p, off)
}

func (r *Reader) Size() uint64 {
	return r.size
}

func (r *Reader) Close() error {
	return r.f.Close()
}
