package imgconv

import (
	"fmt"
	"strings"

	"imgconv/internal/formats/qcow2"
)

func Map(opts MapOptions) (*MapResult, error) {
	if opts.Path == "" {
		return nil, fmt.Errorf("%w: empty path", ErrInvalidArgument)
	}

	format := opts.InputFormat
	if format == FormatAuto {
		lower := strings.ToLower(opts.Path)
		switch {
		case strings.HasSuffix(lower, ".qcow2"):
			format = FormatQCOW2
		case strings.HasSuffix(lower, ".vmdk"):
			format = FormatVMDK
		case strings.HasSuffix(lower, ".vdi"):
			format = FormatVDI
		default:
			format = FormatRAW
		}
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

	default:
		return nil, fmt.Errorf("%w: map currently supports only qcow2", ErrUnsupportedFormat)
	}
}
