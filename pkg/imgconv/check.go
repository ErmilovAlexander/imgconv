package imgconv

import (
	"fmt"

	"github.com/ErmilovAlexander/imgconv/internal/formats/vdi"
	"github.com/ErmilovAlexander/imgconv/internal/formats/vmdk"
	"github.com/ErmilovAlexander/imgconv/internal/image"
	"github.com/ErmilovAlexander/imgconv/internal/pipeline"
)

func Check(path string, opts CheckOptions) (*CheckResult, error) {
	if path == "" {
		return nil, fmt.Errorf("%w: empty path", ErrInvalidArgument)
	}

	fmtHint, err := image.ParseFormat(string(opts.InputFormat))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidArgument, err)
	}
	if fmtHint == "" {
		fmtHint = image.DetectFormat(path)
	}

	switch fmtHint {
	case image.FormatQCOW2:
		if err := pipeline.CheckQCOW2(path); err != nil {
			return nil, fmt.Errorf("%w: qcow2 check %q: %v", ErrOperationFailed, path, err)
		}
		info, err := Inspect(path, InspectOptions{InputFormat: FormatQCOW2})
		if err != nil {
			return nil, err
		}
		return &CheckResult{
			Path:        path,
			Format:      FormatQCOW2,
			VirtualSize: info.VirtualSize,
			Status:      "OK",
		}, nil

	case image.FormatVMDK:
		r, err := vmdk.Open(path)
		if err != nil {
			return nil, fmt.Errorf("%w: vmdk open %q: %v", ErrOperationFailed, path, err)
		}
		defer r.Close()

		return &CheckResult{
			Path:        path,
			Format:      FormatVMDK,
			VirtualSize: r.Size(),
			Status:      "OK",
		}, nil

	case image.FormatVDI:
		if err := pipeline.CheckVDI(path); err != nil {
			return nil, fmt.Errorf("%w: vdi check %q: %v", ErrOperationFailed, path, err)
		}
		r, err := vdi.Open(path)
		if err != nil {
			return nil, fmt.Errorf("%w: vdi open %q: %v", ErrOperationFailed, path, err)
		}
		defer r.Close()

		return &CheckResult{
			Path:        path,
			Format:      FormatVDI,
			VirtualSize: r.Size(),
			Status:      "OK",
		}, nil

	default:
		return nil, fmt.Errorf("%w: check supports qcow2, vmdk and vdi", ErrUnsupportedFormat)
	}
}
