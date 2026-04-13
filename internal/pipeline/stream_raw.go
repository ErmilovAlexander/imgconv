package pipeline

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"
)

type StreamRawOptions struct {
	ChunkSize      uint64
	ProgressWriter io.Writer
	ProgressFunc   ProgressCallback
}

func ConvertToRawWriter(ctx context.Context, in RangeReader, out io.Writer, opts StreamRawOptions) error {
	if out == nil {
		return fmt.Errorf("convert: nil output writer")
	}
	if opts.ChunkSize == 0 {
		opts.ChunkSize = 4 << 20
	}
	if opts.ProgressWriter == nil {
		opts.ProgressWriter = os.Stderr
	}

	size := in.Size()
	pg := NewProgress(size, opts.ProgressFunc)

	doneCh := make(chan struct{})
	go func() {
		t := time.NewTicker(200 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				pg.Render(opts.ProgressWriter, false)
			case <-doneCh:
				pg.Render(opts.ProgressWriter, true)
				if opts.ProgressWriter != nil {
					fmt.Fprint(opts.ProgressWriter, "\n")
				}
				return
			}
		}
	}()
	defer close(doneCh)

	buf := make([]byte, opts.ChunkSize)

	for off := uint64(0); off < size; {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		want := opts.ChunkSize
		if off+want > size {
			want = size - off
		}

		chunk := buf[:want]
		n, err := in.ReadAt(chunk, int64(off))
		if err != nil && err != io.EOF {
			return fmt.Errorf("readat off=%d: %w", off, err)
		}
		if n == 0 {
			break
		}

		written := 0
		for written < n {
			wn, werr := out.Write(chunk[written:n])
			if werr != nil {
				return fmt.Errorf("write raw stream off=%d: %w", off+uint64(written), werr)
			}
			if wn <= 0 {
				return io.ErrShortWrite
			}
			written += wn
		}

		off += uint64(n)
		pg.AddDone(uint64(n))
	}

	return nil
}
