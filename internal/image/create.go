package image

import (
	"fmt"

	"github.com/ErmilovAlexander/imgconv/internal/formats/qcow2"
	"github.com/ErmilovAlexander/imgconv/internal/formats/raw"
	"github.com/ErmilovAlexander/imgconv/internal/formats/vdi"
	"github.com/ErmilovAlexander/imgconv/internal/formats/vmdk"
	"github.com/ErmilovAlexander/imgconv/internal/pipeline"
)

type CreateOptions struct {
	Size        uint64
	Sparse      bool
	ClusterBits uint32
	BlockSize   uint32
	BackingFile string
}

func Create(path string, format Format, opts CreateOptions) (pipeline.RangeWriter, error) {
	if opts.Size == 0 {
		return nil, fmt.Errorf("create: size must be > 0")
	}

	switch format {
	case FormatRAW:
		return raw.Create(path, opts.Size, raw.Options{
			Sparse: opts.Sparse,
		})

	case FormatQCOW2:
		return qcow2.Create(path, opts.Size, qcow2.WriterOptions{
			ClusterBits: opts.ClusterBits,
			Sparse:      opts.Sparse,
			BackingFile: opts.BackingFile,
		})

	case FormatVDI:
		return vdi.Create(path, opts.Size, vdi.WriterOptions{
			BlockSize: opts.BlockSize,
			Sparse:    opts.Sparse,
		})

	case FormatVMDK:
		return vmdk.Create(path, opts.Size, vmdk.WriterOptions{
			Sparse: opts.Sparse,
		})

	default:
		return nil, fmt.Errorf("create: unsupported format %q", format)
	}
}
