package image

import (
	"fmt"
	"os"

	"github.com/ErmilovAlexander/imgconv/internal/formats/qcow2"
	"github.com/ErmilovAlexander/imgconv/internal/formats/vdi"
	"github.com/ErmilovAlexander/imgconv/internal/formats/vmdk"
)

type Info struct {
	Path        string         `json:"path"`
	Format      Format         `json:"format"`
	VirtualSize uint64         `json:"virtual_size"`
	FileSize    uint64         `json:"file_size"`
	Details     map[string]any `json:"details,omitempty"`
}

func Inspect(path string, hint string) (*Info, error) {
	f, err := ParseFormat(hint)
	if err != nil {
		return nil, err
	}
	if f == "" {
		f = DetectFormat(path)
	}

	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	info := &Info{
		Path:     path,
		Format:   f,
		FileSize: uint64(st.Size()),
		Details:  map[string]any{},
	}

	switch f {
	case FormatRAW:
		info.VirtualSize = uint64(st.Size())

	case FormatQCOW2:
		r, err := qcow2.Open(path)
		if err != nil {
			return nil, err
		}
		defer r.Close()

		info.VirtualSize = r.Size()
		info.Details["cluster_bits"] = r.ClusterBits()
		info.Details["cluster_size"] = uint64(1) << r.ClusterBits()
		info.Details["l1_size"] = r.L1Size()
		info.Details["compression_type"] = r.CompressionType()
		if bf := r.BackingFile(); bf != "" {
			info.Details["backing_file"] = bf
		}

	case FormatVMDK:
		r, err := vmdk.Open(path)
		if err != nil {
			return nil, err
		}
		defer r.Close()

		info.VirtualSize = r.Size()

	case FormatVDI:
		r, err := vdi.Open(path)
		if err != nil {
			return nil, err
		}
		defer r.Close()

		info.VirtualSize = r.Size()
		info.Details["block_size"] = r.BlockSize()
		info.Details["image_type"] = r.ImageType()

	default:
		return nil, fmt.Errorf("unsupported format %q", f)
	}

	return info, nil
}
