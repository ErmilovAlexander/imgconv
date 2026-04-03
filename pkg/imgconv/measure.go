package imgconv

import (
	"fmt"

	"imgconv/internal/formats/qcow2"
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

	default:
		return nil, fmt.Errorf("%w: measure currently supports only qcow2", ErrUnsupportedFormat)
	}
}
