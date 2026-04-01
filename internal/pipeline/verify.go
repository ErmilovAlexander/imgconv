package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"imgconv/internal/formats/qcow2"
	"imgconv/internal/formats/raw"
	"imgconv/internal/formats/vdi"
	"imgconv/internal/formats/vmdk"
)

type VerifyMode string

const (
	VerifyNone   VerifyMode = "none"
	VerifySample VerifyMode = "sample"
	VerifyFull   VerifyMode = "full"
)

type VerifyOptions struct {
	Mode         VerifyMode
	SampleBlocks int
	ChunkSize    uint64
}

func VerifyRange(ctx context.Context, reopenSrc func() (RangeReader, error), dstPath, dstFormat string, opts VerifyOptions) error {
	if opts.Mode == VerifyNone {
		return nil
	}
	if opts.ChunkSize == 0 {
		opts.ChunkSize = 4 << 20
	}
	if opts.SampleBlocks <= 0 {
		opts.SampleBlocks = 128
	}

	src, err := reopenSrc()
	if err != nil {
		return err
	}
	defer src.Close()

	var dst RangeReader
	switch dstFormat {
	case "raw":
		dst, err = raw.Open(dstPath)
	case "qcow2":
		dst, err = qcow2.Open(dstPath)
	case "vdi":
		dst, err = vdi.Open(dstPath)
	case "vmdk":
		dst, err = vmdk.Open(dstPath)
	default:
		return fmt.Errorf("verify: unsupported dst format %q", dstFormat)
	}
	if err != nil {
		return err
	}
	defer dst.Close()

	if src.Size() != dst.Size() {
		return fmt.Errorf("verify: size mismatch src=%d dst=%d", src.Size(), dst.Size())
	}

	switch opts.Mode {
	case VerifyFull:
		return verifyFull(ctx, src, dst, opts.ChunkSize)
	case VerifySample:
		return verifySample(ctx, src, dst, opts.SampleBlocks, opts.ChunkSize)
	default:
		return nil
	}
}

func verifyFull(ctx context.Context, src, dst RangeReader, chunkSize uint64) error {
	size := src.Size()
	srcBuf := make([]byte, chunkSize)
	dstBuf := make([]byte, chunkSize)

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

		if _, err := readFullAt(src, srcBuf[:want], int64(off)); err != nil && err != io.EOF {
			return fmt.Errorf("verify src at %d: %w", off, err)
		}
		if _, err := readFullAt(dst, dstBuf[:want], int64(off)); err != nil && err != io.EOF {
			return fmt.Errorf("verify dst at %d: %w", off, err)
		}
		if !bytes.Equal(srcBuf[:want], dstBuf[:want]) {
			return fmt.Errorf("verify mismatch at offset %d", off)
		}
	}

	return nil
}

func verifySample(ctx context.Context, src, dst RangeReader, blocks int, chunkSize uint64) error {
	size := src.Size()
	if size == 0 {
		return nil
	}
	if uint64(blocks)*chunkSize >= size {
		return verifyFull(ctx, src, dst, chunkSize)
	}

	srcBuf := make([]byte, chunkSize)
	dstBuf := make([]byte, chunkSize)

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

		if _, err := readFullAt(src, srcBuf[:want], int64(off)); err != nil && err != io.EOF {
			return fmt.Errorf("verify src sample at %d: %w", off, err)
		}
		if _, err := readFullAt(dst, dstBuf[:want], int64(off)); err != nil && err != io.EOF {
			return fmt.Errorf("verify dst sample at %d: %w", off, err)
		}
		if !bytes.Equal(srcBuf[:want], dstBuf[:want]) {
			return fmt.Errorf("verify sample mismatch at offset %d", off)
		}
	}

	return nil
}

func readFullAt(r RangeReader, p []byte, off int64) (int, error) {
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
