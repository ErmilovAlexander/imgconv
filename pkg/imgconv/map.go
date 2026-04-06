package imgconv

import (
	"fmt"

	"github.com/ErmilovAlexander/imgconv/internal/formats/qcow2"
	"github.com/ErmilovAlexander/imgconv/internal/image"
)

func Map(opts MapOptions) (*MapResult, error) {
	if opts.Path == "" {
		return nil, fmt.Errorf("%w: empty path", ErrInvalidArgument)
	}

	format := opts.InputFormat
	if format == FormatAuto {
		format = Format(image.DetectFormat(opts.Path))
	}

	switch format {
	case FormatQCOW2:
		exts, err := qcow2.MapFile(opts.Path)
		if err != nil {
			return nil, fmt.Errorf("%w: map %q: %v", ErrOperationFailed, opts.Path, err)
		}

		out := make([]MapExtent, 0, len(exts))
		for _, e := range exts {
			out = append(out, MapExtent{
				Start:  e.Start,
				Length: e.Length,
				Kind:   string(e.Kind),
			})
		}

		return &MapResult{
			Format:  FormatQCOW2,
			Path:    opts.Path,
			Extents: out,
		}, nil

	case FormatRAW, FormatVDI, FormatVMDK:
		r, err := image.Open(opts.Path, string(format))
		if err != nil {
			return nil, fmt.Errorf("%w: map %q: %v", ErrOperationFailed, opts.Path, err)
		}
		defer r.Reader.Close()
		return &MapResult{
			Format: format,
			Path:   opts.Path,
			Extents: []MapExtent{
				{
					Start:  0,
					Length: r.Size,
					Kind:   "data",
				},
			},
		}, nil

	default:
		return nil, fmt.Errorf("%w: map supports qcow2, raw, vdi and vmdk", ErrUnsupportedFormat)
	}
}
