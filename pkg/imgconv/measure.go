package imgconv

import (
	"fmt"

	"github.com/ErmilovAlexander/imgconv/internal/formats/qcow2"
)

func Measure(opts MeasureOptions) (*MeasureResult, error) {
	if opts.Format == FormatAuto {
		return nil, fmt.Errorf("%w: format is required", ErrInvalidArgument)
	}
	if opts.Size == 0 {
		return nil, fmt.Errorf("%w: size must be > 0", ErrInvalidArgument)
	}

	switch opts.Format {
	case FormatQCOW2:
		m, err := qcow2.Measure(qcow2.MeasureOptions{
			Size:        opts.Size,
			ClusterBits: opts.ClusterBits,
			BackingFile: opts.BackingFile,
		})
		if err != nil {
			return nil, fmt.Errorf("%w: measure qcow2: %v", ErrOperationFailed, err)
		}

		return &MeasureResult{
			Format:                FormatQCOW2,
			VirtualSize:           m.VirtualSize,
			ClusterBits:           m.ClusterBits,
			ClusterSize:           m.ClusterSize,
			BlockSize:             0,
			L1Entries:             m.L1Entries,
			L1Clusters:            m.L1Clusters,
			MaxDataClusters:       m.MaxDataClusters,
			MaxL2Clusters:         m.MaxL2Clusters,
			RefcountBlockEntries:  m.RefcountBlockEntries,
			RefcountBlockCount:    m.RefcountBlockCount,
			RefcountTableClusters: m.RefcountTableClusters,
			MetadataClusters:      m.MetadataClusters,
			MetadataBytes:         m.MetadataBytes,
			BackingFile:           m.BackingFile,
		}, nil

	case FormatRAW:
		return &MeasureResult{
			Format:          FormatRAW,
			VirtualSize:     opts.Size,
			ClusterBits:     0,
			ClusterSize:     0,
			BlockSize:       0,
			MetadataClusters: 0,
			MetadataBytes:   0,
		}, nil

	case FormatVDI:
		blockSize := opts.BlockSize
		if blockSize == 0 {
			blockSize = 1 << 20
		}
		if blockSize%4096 != 0 {
			return nil, fmt.Errorf("%w: vdi block size must be multiple of 4096", ErrInvalidArgument)
		}
		blocks := opts.Size / uint64(blockSize)
		if opts.Size%uint64(blockSize) != 0 {
			blocks++
		}
		entriesBytes := blocks * 4
		dataOffset := uint64(512) + entriesBytes
		if rem := dataOffset % 4096; rem != 0 {
			dataOffset += 4096 - rem
		}
		metadataBytes := dataOffset

		return &MeasureResult{
			Format:        FormatVDI,
			VirtualSize:   opts.Size,
			ClusterBits:   0,
			ClusterSize:   0,
			BlockSize:     blockSize,
			L1Entries:     0,
			L1Clusters:    0,
			MetadataBytes: metadataBytes,
		}, nil

	default:
		return nil, fmt.Errorf("%w: measure currently supports qcow2, raw and vdi", ErrUnsupportedFormat)
	}
}
