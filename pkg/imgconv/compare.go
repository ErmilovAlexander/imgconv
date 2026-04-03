package imgconv

import (
	"context"
	"fmt"

	"imgconv/internal/ops"
	"imgconv/internal/pipeline"
)

func Compare(ctx context.Context, opts CompareOptions) error {
	if opts.LeftPath == "" || opts.RightPath == "" {
		return fmt.Errorf("%w: both left and right paths are required", ErrInvalidArgument)
	}

	mode := pipeline.VerifyMode(opts.Mode)
	if mode == "" {
		mode = pipeline.VerifyFull
	}

	if err := ops.ComparePaths(ctx, opts.LeftPath, string(opts.LeftFormat), opts.RightPath, string(opts.RightFormat), ops.CompareOptions{
		Mode:         mode,
		SampleBlocks: opts.SampleBlocks,
		ChunkSize:    opts.ChunkSize,
	}); err != nil {
		return fmt.Errorf("%w: compare %q vs %q: %v", ErrOperationFailed, opts.LeftPath, opts.RightPath, err)
	}

	return nil
}
