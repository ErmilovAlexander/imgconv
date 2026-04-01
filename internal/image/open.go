package image

import (
	"fmt"
	"path/filepath"
	"strings"

	"imgconv/internal/formats/qcow2"
	"imgconv/internal/formats/raw"
	"imgconv/internal/formats/vdi"
	"imgconv/internal/formats/vmdk"
	"imgconv/internal/pipeline"
)

type Format string

const (
	FormatRAW   Format = "raw"
	FormatQCOW2 Format = "qcow2"
	FormatVMDK  Format = "vmdk"
	FormatVDI   Format = "vdi"
)

type OpenResult struct {
	Reader pipeline.RangeReader
	Format Format
	Size   uint64
}

func ParseFormat(s string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "auto":
		return "", nil
	case string(FormatRAW):
		return FormatRAW, nil
	case string(FormatQCOW2):
		return FormatQCOW2, nil
	case string(FormatVMDK):
		return FormatVMDK, nil
	case string(FormatVDI):
		return FormatVDI, nil
	default:
		return "", fmt.Errorf("unsupported format %q", s)
	}
}

func DetectFormat(path string) Format {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".qcow2":
		return FormatQCOW2
	case ".vmdk":
		return FormatVMDK
	case ".vdi":
		return FormatVDI
	case ".raw", ".img", ".bin":
		return FormatRAW
	default:
		return FormatRAW
	}
}

func Open(path string, hint string) (*OpenResult, error) {
	f, err := ParseFormat(hint)
	if err != nil {
		return nil, err
	}
	if f == "" {
		f = DetectFormat(path)
	}

	switch f {
	case FormatRAW:
		r, err := raw.Open(path)
		if err != nil {
			return nil, err
		}
		return &OpenResult{Reader: r, Format: f, Size: r.Size()}, nil

	case FormatQCOW2:
		r, err := qcow2.Open(path)
		if err != nil {
			return nil, err
		}
		return &OpenResult{Reader: r, Format: f, Size: r.Size()}, nil

	case FormatVMDK:
		r, err := vmdk.Open(path)
		if err != nil {
			return nil, err
		}
		return &OpenResult{Reader: r, Format: f, Size: r.Size()}, nil

	case FormatVDI:
		r, err := vdi.Open(path)
		if err != nil {
			return nil, err
		}
		return &OpenResult{Reader: r, Format: f, Size: r.Size()}, nil

	default:
		return nil, fmt.Errorf("unsupported input format %q", f)
	}
}
