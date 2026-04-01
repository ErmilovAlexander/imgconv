package ops

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"imgconv/internal/formats/qcow2"
	"imgconv/internal/image"
	"imgconv/internal/pipeline"
)

type CommitOptions struct {
	ChunkSize uint64
	Sparse    bool
}

func CommitQCOW2Overlay(ctx context.Context, overlayPath string, opts CommitOptions) error {
	if opts.ChunkSize == 0 {
		opts.ChunkSize = 4 << 20
	}

	ovr, err := qcow2.Open(overlayPath)
	if err != nil {
		return fmt.Errorf("open overlay: %w", err)
	}
	defer ovr.Close()

	backingRel := ovr.BackingFile()
	if backingRel == "" {
		return fmt.Errorf("commit: overlay has no backing file")
	}

	backingPath := backingRel
	if !filepath.IsAbs(backingRel) {
		backingPath = filepath.Join(filepath.Dir(overlayPath), backingRel)
	}

	backingFmt := image.DetectFormat(backingPath)
	switch backingFmt {
	case image.FormatRAW, image.FormatQCOW2, image.FormatVDI:
	default:
		return fmt.Errorf("commit: backing format %q is not supported for commit", backingFmt)
	}

	src, err := image.Open(overlayPath, "qcow2")
	if err != nil {
		return fmt.Errorf("open overlay for copy: %w", err)
	}
	defer src.Reader.Close()

	tmpBacking := backingPath + ".imgconv-commit-tmp"
	_ = os.Remove(tmpBacking)

	dst, err := image.Create(tmpBacking, backingFmt, image.CreateOptions{
		Size:   src.Size,
		Sparse: opts.Sparse,
	})
	if err != nil {
		return fmt.Errorf("create temp backing: %w", err)
	}
	if err := copyReaderToWriter(ctx, src.Reader, dst, opts.ChunkSize); err != nil {
		_ = dst.Close()
		_ = os.Remove(tmpBacking)
		return fmt.Errorf("commit copy: %w", err)
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(tmpBacking)
		return fmt.Errorf("close temp backing: %w", err)
	}

	if err := ComparePaths(ctx, overlayPath, "qcow2", tmpBacking, string(backingFmt), CompareOptions{
		Mode:      pipeline.VerifyFull,
		ChunkSize: opts.ChunkSize,
	}); err != nil {
		_ = os.Remove(tmpBacking)
		return fmt.Errorf("commit verify temp backing: %w", err)
	}

	if err := os.Rename(tmpBacking, backingPath); err != nil {
		_ = os.Remove(tmpBacking)
		return fmt.Errorf("replace backing: %w", err)
	}

	// Reset overlay to an empty overlay pointing to the same backing path.
	tmpOverlay := overlayPath + ".imgconv-reset-tmp"
	_ = os.Remove(tmpOverlay)

	newOverlay, err := qcow2.Create(tmpOverlay, src.Size, qcow2.WriterOptions{
		ClusterBits: ovr.ClusterBits(),
		Sparse:      true,
		BackingFile: backingRel,
	})
	if err != nil {
		_ = os.Remove(tmpOverlay)
		return fmt.Errorf("recreate overlay: %w", err)
	}
	if err := newOverlay.Close(); err != nil {
		_ = os.Remove(tmpOverlay)
		return fmt.Errorf("close recreated overlay: %w", err)
	}

	if err := os.Rename(tmpOverlay, overlayPath); err != nil {
		_ = os.Remove(tmpOverlay)
		return fmt.Errorf("replace overlay: %w", err)
	}

	return nil
}

func copyReaderToWriter(ctx context.Context, src pipeline.RangeReader, dst pipeline.RangeWriter, chunkSize uint64) error {
	size := src.Size()
	buf := make([]byte, chunkSize)

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

		n, err := readFullAt(src, buf[:want], int64(off))
		if err != nil && err != io.EOF {
			return err
		}
		if n == 0 {
			continue
		}
		if _, err := dst.WriteAt(buf[:n], int64(off)); err != nil {
			return err
		}
	}

	return nil
}
