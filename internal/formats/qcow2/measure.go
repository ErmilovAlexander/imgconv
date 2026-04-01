package qcow2

import "fmt"

type MeasureOptions struct {
	Size        uint64
	ClusterBits uint32
	BackingFile string
}

type MeasureResult struct {
	Format                string `json:"format"`
	VirtualSize           uint64 `json:"virtual_size"`
	ClusterBits           uint32 `json:"cluster_bits"`
	ClusterSize           uint64 `json:"cluster_size"`
	L1Entries             uint32 `json:"l1_entries"`
	L1Clusters            uint64 `json:"l1_clusters"`
	MaxDataClusters       uint64 `json:"max_data_clusters"`
	MaxL2Clusters         uint64 `json:"max_l2_clusters"`
	RefcountBlockEntries  uint64 `json:"refcount_block_entries"`
	RefcountBlockCount    uint64 `json:"refcount_block_count"`
	RefcountTableClusters uint64 `json:"refcount_table_clusters"`
	MetadataClusters      uint64 `json:"metadata_clusters"`
	MetadataBytes         uint64 `json:"metadata_bytes"`
	BackingFile           string `json:"backing_file,omitempty"`
}

func Measure(opts MeasureOptions) (*MeasureResult, error) {
	if opts.Size == 0 {
		return nil, fmt.Errorf("measure: size must be > 0")
	}

	clusterBits := opts.ClusterBits
	if clusterBits == 0 {
		clusterBits = 16
	}
	clusterSize := uint64(1) << clusterBits
	if clusterSize < 4096 {
		return nil, fmt.Errorf("measure: cluster size too small")
	}

	epl2 := entriesPerL2(clusterSize)
	l1Size := uint32((opts.Size + clusterSize*epl2 - 1) / (clusterSize * epl2))
	if l1Size == 0 {
		l1Size = 1
	}

	l1Bytes := uint64(l1Size) * 8
	l1Clusters := ceilDiv(l1Bytes, clusterSize)
	if l1Clusters == 0 {
		l1Clusters = 1
	}

	maxDataClusters := ceilDiv(opts.Size, clusterSize)
	maxL2Clusters := ceilDiv(maxDataClusters, epl2)

	refcountBlockEntries := clusterSize / 2
	if refcountBlockEntries == 0 {
		return nil, fmt.Errorf("measure: invalid refcount block geometry")
	}

	refcountTableClusters := uint64(1)
	refcountBlockCount := uint64(1)
	for {
		refcountTableClusters = ceilDiv(refcountBlockCount*8, clusterSize)
		if refcountTableClusters == 0 {
			refcountTableClusters = 1
		}

		metadataClusters := uint64(1) +
			refcountTableClusters +
			refcountBlockCount +
			l1Clusters +
			maxL2Clusters

		totalClustersMax := metadataClusters + maxDataClusters
		newRefcountBlockCount := ceilDiv(totalClustersMax, refcountBlockEntries)
		if newRefcountBlockCount == 0 {
			newRefcountBlockCount = 1
		}
		if newRefcountBlockCount == refcountBlockCount {
			break
		}
		refcountBlockCount = newRefcountBlockCount
	}

	metadataClusters := uint64(1) +
		refcountTableClusters +
		refcountBlockCount +
		l1Clusters +
		maxL2Clusters

	return &MeasureResult{
		Format:                "qcow2",
		VirtualSize:           opts.Size,
		ClusterBits:           clusterBits,
		ClusterSize:           clusterSize,
		L1Entries:             l1Size,
		L1Clusters:            l1Clusters,
		MaxDataClusters:       maxDataClusters,
		MaxL2Clusters:         maxL2Clusters,
		RefcountBlockEntries:  refcountBlockEntries,
		RefcountBlockCount:    refcountBlockCount,
		RefcountTableClusters: refcountTableClusters,
		MetadataClusters:      metadataClusters,
		MetadataBytes:         metadataClusters * clusterSize,
		BackingFile:           opts.BackingFile,
	}, nil
}
