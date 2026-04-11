package imgconv

import (
	"context"
	"fmt"

	"github.com/ErmilovAlexander/imgconv/internal/image"
	"github.com/ErmilovAlexander/imgconv/internal/pipeline"
)

func Convert(ctx context.Context, opts ConvertOptions) error {
	if opts.InputPath == "" {
		return fmt.Errorf("%w: empty input path", ErrInvalidArgument)
	}
	if opts.OutputPath == "" {
		return fmt.Errorf("%w: empty output path", ErrInvalidArgument)
	}
	if opts.OutputFormat == FormatAuto {
		return fmt.Errorf("%w: output format is required", ErrInvalidArgument)
	}

	src, err := image.Open(opts.InputPath, string(opts.InputFormat))
	if err != nil {
		return fmt.Errorf("%w: open input %q: %v", ErrOperationFailed, opts.InputPath, err)
	}
	defer src.Reader.Close()

	reopen := func() (pipeline.RangeReader, error) {
		res, err := image.Open(opts.InputPath, string(opts.InputFormat))
		if err != nil {
			return nil, err
		}
		return res.Reader, nil
	}

	if err := pipeline.ConvertRange(ctx, src.Reader, opts.OutputPath, pipeline.ConvertRangeOptions{
		Threads:        opts.Threads,
		Sparse:         opts.Sparse,
		ChunkSize:      opts.ChunkSize,
		ClusterBits:    opts.ClusterBits,
		BlockSize:      opts.BlockSize,
		ProgressWriter: opts.ProgressWriter,
		ProgressFunc: func(done, total uint64, percent float64) {
			if opts.Progress != nil {
				opts.Progress(ProgressInfo{
					DoneBytes:  done,
					TotalBytes: total,
					Percent:    percent,
				})
			}
		},
		Format: string(opts.OutputFormat),
	}); err != nil {
		return fmt.Errorf("%w: convert %q -> %q: %v", ErrOperationFailed, opts.InputPath, opts.OutputPath, err)
	}

	vm := pipeline.VerifyMode(opts.VerifyMode)
	if vm == "" {
		vm = pipeline.VerifyNone
	}

	if vm != pipeline.VerifyNone {
		samples := opts.VerifySamples
		if samples <= 0 {
			samples = 256
		}
		if err := pipeline.VerifyRange(ctx, reopen, opts.OutputPath, string(opts.OutputFormat), pipeline.VerifyOptions{
			Mode:         vm,
			SampleBlocks: samples,
			ChunkSize:    opts.ChunkSize,
		}); err != nil {
			return fmt.Errorf("%w: verify output %q: %v", ErrOperationFailed, opts.OutputPath, err)
		}
	}

	return nil
}

func ConvertToRawWriter(ctx context.Context, opts ConvertToRawWriterOptions) error {
	if opts.InputPath == "" {
		return fmt.Errorf("%w: empty input path", ErrInvalidArgument)
	}
	if opts.Output == nil {
		return fmt.Errorf("%w: nil output writer", ErrInvalidArgument)
	}

	src, err := image.Open(opts.InputPath, string(opts.InputFormat))
	if err != nil {
		return fmt.Errorf("%w: open input %q: %v", ErrOperationFailed, opts.InputPath, err)
	}
	defer src.Reader.Close()

	if err := pipeline.ConvertToRawWriter(ctx, src.Reader, opts.Output, pipeline.StreamRawOptions{
		ChunkSize:      opts.ChunkSize,
		ProgressWriter: opts.ProgressWriter,
		ProgressFunc: func(done, total uint64, percent float64) {
			if opts.Progress != nil {
				opts.Progress(ProgressInfo{
					DoneBytes:  done,
					TotalBytes: total,
					Percent:    percent,
				})
			}
		},
	}); err != nil {
		return fmt.Errorf("%w: convert %q -> raw stream: %v", ErrOperationFailed, opts.InputPath, err)
	}

	return nil
}
