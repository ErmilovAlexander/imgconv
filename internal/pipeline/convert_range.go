package pipeline

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/ErmilovAlexander/imgconv/internal/formats/qcow2"
	"github.com/ErmilovAlexander/imgconv/internal/formats/raw"
	"github.com/ErmilovAlexander/imgconv/internal/formats/vdi"
	"github.com/ErmilovAlexander/imgconv/internal/formats/vmdk"
)

type ConvertRangeOptions struct {
	Threads        int
	Sparse         bool
	ChunkSize      uint64
	ClusterBits    uint32
	BlockSize      uint32
	ProgressWriter io.Writer
	ProgressFunc   ProgressCallback
	Format         string
}

func ConvertRange(ctx context.Context, in RangeReader, outPath string, opts ConvertRangeOptions) error {
	if opts.Threads <= 0 {
		opts.Threads = runtime.NumCPU()
	}
	if opts.ProgressWriter == nil {
		opts.ProgressWriter = os.Stderr
	}
	if opts.ChunkSize == 0 {
		opts.ChunkSize = 4 << 20
	}
	if opts.ClusterBits == 0 {
		opts.ClusterBits = 16
	}
	if opts.BlockSize == 0 {
		opts.BlockSize = 1 << 20
	}
	if opts.Format == "" {
		opts.Format = "raw"
	}

	var out RangeWriter
	var outClose func() error

	switch opts.Format {
	case "raw":
		w, err := raw.Create(outPath, in.Size(), raw.Options{Sparse: opts.Sparse})
		if err != nil {
			return err
		}
		out = w
		outClose = w.Close

	case "qcow2":
		w, err := qcow2.Create(outPath, in.Size(), qcow2.WriterOptions{
			ClusterBits: opts.ClusterBits,
			Sparse:      opts.Sparse,
		})
		if err != nil {
			return err
		}
		out = w
		outClose = w.Close

	case "vdi":
		w, err := vdi.Create(outPath, in.Size(), vdi.WriterOptions{
			BlockSize: opts.BlockSize,
			Sparse:    opts.Sparse,
		})
		if err != nil {
			return err
		}
		out = w
		outClose = w.Close

	case "vmdk":
		w, err := vmdk.Create(outPath, in.Size(), vmdk.WriterOptions{
			Sparse: opts.Sparse,
		})
		if err != nil {
			return err
		}
		out = w
		outClose = w.Close

	default:
		return fmt.Errorf("convert: unsupported format %q", opts.Format)
	}
	defer outClose()

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

	type job struct {
		off  uint64
		want uint64
	}
	type writeJob struct {
		off uint64
		buf []byte
		n   int
	}

	jobs := make(chan job, opts.Threads*4)
	writes := make(chan writeJob, opts.Threads*2)
	errCh := make(chan error, 1)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	setErr := func(e error) {
		select {
		case errCh <- e:
			cancel()
		default:
		}
	}

	var pool = sync.Pool{
		New: func() any {
			return make([]byte, opts.ChunkSize)
		},
	}

	var writeWG sync.WaitGroup
	writeWG.Add(1)
	go func() {
		defer writeWG.Done()
		for wj := range writes {
			_, e := out.WriteAt(wj.buf[:wj.n], int64(wj.off))
			if e != nil {
				pool.Put(wj.buf)
				setErr(fmt.Errorf("writeat off=%d: %w", wj.off, e))
				return
			}
			pg.AddDone(uint64(wj.n))
			pool.Put(wj.buf)
		}
	}()

	var wg sync.WaitGroup
	worker := func() {
		defer wg.Done()

		for j := range jobs {
			select {
			case <-ctx.Done():
				return
			default:
			}

			buf := pool.Get().([]byte)
			b := buf[:j.want]

			n, e := in.ReadAt(b, int64(j.off))
			if e != nil && e != io.EOF {
				pool.Put(buf)
				setErr(fmt.Errorf("readat off=%d: %w", j.off, e))
				return
			}
			b = b[:n]
			if n == 0 {
				pool.Put(buf)
				continue
			}

			if opts.Format == "raw" && opts.Sparse && isAllZero(b) {
				pg.AddDone(uint64(n))
				pool.Put(buf)
				continue
			}

			select {
			case <-ctx.Done():
				pool.Put(buf)
				return
			case writes <- writeJob{
				off: j.off,
				buf: buf,
				n:   n,
			}:
			}
		}
	}

	wg.Add(opts.Threads)
	for i := 0; i < opts.Threads; i++ {
		go worker()
	}

	go func() {
		for off := uint64(0); off < size; off += opts.ChunkSize {
			want := opts.ChunkSize
			if off+want > size {
				want = size - off
			}
			select {
			case <-ctx.Done():
				close(jobs)
				return
			case jobs <- job{off: off, want: want}:
			}
		}
		close(jobs)
	}()

	wg.Wait()
	close(writes)
	writeWG.Wait()

	select {
	case e := <-errCh:
		return e
	default:
		return nil
	}
}

func isAllZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}
