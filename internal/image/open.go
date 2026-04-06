package image

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ErmilovAlexander/imgconv/internal/formats/qcow2"
	"github.com/ErmilovAlexander/imgconv/internal/formats/raw"
	"github.com/ErmilovAlexander/imgconv/internal/formats/vdi"
	"github.com/ErmilovAlexander/imgconv/internal/formats/vmdk"
	"github.com/ErmilovAlexander/imgconv/internal/pipeline"
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
	if f := detectFormatByHeader(path); f != "" {
		return f
	}

	return detectFormatByExtension(path)
}

func detectFormatByExtension(path string) Format {
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

func detectFormatByHeader(path string) Format {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	buf := make([]byte, 1024)
	n, err := f.Read(buf)
	if err != nil || n == 0 {
		return ""
	}
	buf = buf[:n]

	if len(buf) >= 4 && binary.BigEndian.Uint32(buf[:4]) == 0x514649fb {
		return FormatQCOW2
	}
	if len(buf) >= 68 && binary.LittleEndian.Uint32(buf[64:68]) == 0xBEDA107F {
		return FormatVDI
	}
	if len(buf) >= 4 && binary.LittleEndian.Uint32(buf[:4]) == 0x564D444B {
		return FormatVMDK
	}
	if bytes.Contains(bytes.ToLower(buf), []byte("disk descriptorfile")) {
		return FormatVMDK
	}

	return ""
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
