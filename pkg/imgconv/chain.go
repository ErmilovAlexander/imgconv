package imgconv

import (
	"context"
	"fmt"

	"imgconv/internal/formats/qcow2"
	"imgconv/internal/ops"
)

func Commit(ctx context.Context, opts CommitOptions) error {
	if opts.OverlayPath == "" {
		return fmt.Errorf("%w: empty overlay path", ErrInvalidArgument)
	}

	if err := ops.CommitQCOW2Overlay(ctx, opts.OverlayPath, ops.CommitOptions{
		ChunkSize: opts.ChunkSize,
		Sparse:    opts.Sparse,
	}); err != nil {
		return fmt.Errorf("%w: commit %q: %v", ErrOperationFailed, opts.OverlayPath, err)
	}

	return nil
}

func Rebase(opts RebaseOptions) error {
	if opts.OverlayPath == "" {
		return fmt.Errorf("%w: empty overlay path", ErrInvalidArgument)
	}

	if err := qcow2.RebasePath(opts.OverlayPath, opts.BackingFile); err != nil {
		return fmt.Errorf("%w: rebase %q: %v", ErrOperationFailed, opts.OverlayPath, err)
	}

	return nil
}
