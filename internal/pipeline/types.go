package pipeline

// RangeReader — минимальный интерфейс входного образа.
type RangeReader interface {
	ReadAt(p []byte, off int64) (int, error)
	Size() uint64
	Close() error
}

// RangeWriter — минимальный интерфейс выхода (raw/qcow2).
type RangeWriter interface {
	WriteAt(p []byte, off int64) (int, error)
	Size() uint64
	Close() error
}
