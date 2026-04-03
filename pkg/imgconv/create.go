package imgconv

import (
	"fmt"

	"imgconv/internal/image"
)

func Create(opts CreateOptions) error {
	if opts.Path == "" {
		return fmt.Errorf("%w: empty output path", ErrInvalidArgument)
	}
	if opts.Format == FormatAuto {
		return fmt.Errorf("%w: output format is required", ErrInvalidArgument)
	}
	if opts.Size == 0 {
		return fmt.Errorf("%w: size must be > 0", ErrInvalidArgument)
	}

	w, err := image.Create(opts.Path, image.Format(opts.Format), image.CreateOptions{
		Size:        opts.Size,
		Sparse:      opts.Sparse,
		ClusterBits: opts.ClusterBits,
		BlockSize:   opts.BlockSize,
		BackingFile: opts.BackingFile,
	})
	if err != nil {
		return fmt.Errorf("%w: create %q: %v", ErrOperationFailed, opts.Path, err)
	}
	defer w.Close()

	return nil
}
