package ops

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"imgconv/internal/image"
	"imgconv/internal/pipeline"
)

type CompareOptions struct {
	Mode         pipeline.VerifyMode
	SampleBlocks int
	ChunkSize    uint64
}

func ComparePaths(ctx context.Context, aPath, aFmt, bPath, bFmt string, opts CompareOptions) error {
	if opts.ChunkSize == 0 {
		opts.ChunkSize = 4 << 20
	}
	if opts.SampleBlocks <= 0 {
		opts.SampleBlocks = 256
	}
	if opts.Mode == "" {
		opts.Mode = pipeline.VerifyFull
	}

	a, err := image.Open(aPath, aFmt)
	if err != nil {
		return fmt.Errorf("open A: %w", err)
	}
	defer a.Reader.Close()

	b, err := image.Open(bPath, bFmt)
	if err != nil {
		return fmt.Errorf("open B: %w", err)
	}
	defer b.Reader.Close()

	return CompareReaders(ctx, a.Reader, b.Reader, opts)
}

func CompareReaders(ctx context.Context, a, b pipeline.RangeReader, opts CompareOptions) error {
	if a.Size() != b.Size() {
		return fmt.Errorf("compare: size mismatch a=%d b=%d", a.Size(), b.Size())
	}

	switch opts.Mode {
	case pipeline.VerifyFull:
		return compareFull(ctx, a, b, opts.ChunkSize)
	case pipeline.VerifySample:
		return compareSample(ctx, a, b, opts.SampleBlocks, opts.ChunkSize)
	case pipeline.VerifyNone:
		return nil
	default:
		return fmt.Errorf("compare: unsupported mode %q", opts.Mode)
	}
}

func compareFull(ctx context.Context, a, b pipeline.RangeReader, chunkSize uint64) error {
	size := a.Size()
	abuf := make([]byte, chunkSize)
	bbuf := make([]byte, chunkSize)

	for off := uint64(0); off < size; off += chunkSize {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		want := chunkSize
		if off+want > size {
			want = size - off
		}

		if _, err := readFullAt(a, abuf[:want], int64(off)); err != nil && err != io.EOF {
			return fmt.Errorf("compare A at %d: %w", off, err)
		}
		if _, err := readFullAt(b, bbuf[:want], int64(off)); err != nil && err != io.EOF {
			return fmt.Errorf("compare B at %d: %w", off, err)
		}

		if !bytes.Equal(abuf[:want], bbuf[:want]) {
			return fmt.Errorf("compare mismatch at offset %d", off)
		}
	}

	return nil
}

func compareSample(ctx context.Context, a, b pipeline.RangeReader, blocks int, chunkSize uint64) error {
	size := a.Size()
	if size == 0 {
		return nil
	}
	if uint64(blocks)*chunkSize >= size {
		return compareFull(ctx, a, b, chunkSize)
	}

	abuf := make([]byte, chunkSize)
	bbuf := make([]byte, chunkSize)

	step := size / uint64(blocks)
	if step == 0 {
		step = chunkSize
	}

	for i := 0; i < blocks; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		off := uint64(i) * step
		if off >= size {
			off = size - 1
		}
		off = (off / chunkSize) * chunkSize

		want := chunkSize
		if off+want > size {
			want = size - off
		}

		if _, err := readFullAt(a, abuf[:want], int64(off)); err != nil && err != io.EOF {
			return fmt.Errorf("compare A sample at %d: %w", off, err)
		}
		if _, err := readFullAt(b, bbuf[:want], int64(off)); err != nil && err != io.EOF {
			return fmt.Errorf("compare B sample at %d: %w", off, err)
		}

		if !bytes.Equal(abuf[:want], bbuf[:want]) {
			return fmt.Errorf("compare sample mismatch at offset %d", off)
		}
	}

	return nil
}

func readFullAt(r pipeline.RangeReader, p []byte, off int64) (int, error) {
	n := 0
	for n < len(p) {
		k, err := r.ReadAt(p[n:], off+int64(n))
		n += k
		if err != nil {
			if err == io.EOF && n == len(p) {
				return n, nil
			}
			return n, err
		}
		if k == 0 {
			break
		}
	}
	if n == len(p) {
		return n, nil
	}
	return n, io.EOF
}
