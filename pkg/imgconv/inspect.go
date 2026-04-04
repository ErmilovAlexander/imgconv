package imgconv

import (
	"fmt"

	"github.com/ErmilovAlexander/imgconv/internal/image"
)

func Inspect(path string, opts InspectOptions) (*Info, error) {
	if path == "" {
		return nil, fmt.Errorf("%w: empty path", ErrInvalidArgument)
	}

	info, err := image.Inspect(path, string(opts.InputFormat))
	if err != nil {
		return nil, fmt.Errorf("%w: inspect %q: %v", ErrOperationFailed, path, err)
	}

	return &Info{
		Path:        info.Path,
		Format:      Format(info.Format),
		VirtualSize: info.VirtualSize,
		FileSize:    info.FileSize,
		Details:     info.Details,
	}, nil
}
