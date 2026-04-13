package imgconv

import "io"

type Format string

const (
	FormatAuto  Format = ""
	FormatRAW   Format = "raw"
	FormatQCOW2 Format = "qcow2"
	FormatVMDK  Format = "vmdk"
	FormatVDI   Format = "vdi"
)

type VerifyMode string

const (
	VerifyNone   VerifyMode = "none"
	VerifySample VerifyMode = "sample"
	VerifyFull   VerifyMode = "full"
)

type ProgressInfo struct {
	DoneBytes  uint64
	TotalBytes uint64
	Percent    float64
}

type ProgressFunc func(ProgressInfo)

type Info struct {
	Path        string         `json:"path"`
	Format      Format         `json:"format"`
	VirtualSize uint64         `json:"virtual_size"`
	FileSize    uint64         `json:"file_size"`
	Details     map[string]any `json:"details,omitempty"`
}

type InspectOptions struct {
	InputFormat Format
}

type CreateOptions struct {
	Path        string
	Format      Format
	Size        uint64
	Sparse      bool
	ClusterBits uint32
	BlockSize   uint32
	BackingFile string
}

type ConvertOptions struct {
	InputPath      string
	OutputPath     string
	InputFormat    Format
	OutputFormat   Format
	Sparse         bool
	Threads        int
	ChunkSize      uint64
	ClusterBits    uint32
	BlockSize      uint32
	VerifyMode     VerifyMode
	VerifySamples  int
	ProgressWriter io.Writer
	Progress       ProgressFunc
}

type ConvertToRawWriterOptions struct {
	InputPath      string
	InputFormat    Format
	Output         io.Writer
	ChunkSize      uint64
	ProgressWriter io.Writer
	Progress       ProgressFunc
}

type CompareOptions struct {
	LeftPath     string
	RightPath    string
	LeftFormat   Format
	RightFormat  Format
	Mode         VerifyMode
	SampleBlocks int
	ChunkSize    uint64
}

type CommitOptions struct {
	OverlayPath string
	ChunkSize   uint64
	Sparse      bool
}

type RebaseOptions struct {
	OverlayPath string
	BackingFile string
}

type CheckOptions struct {
	InputFormat Format
}

type CheckResult struct {
	Path        string `json:"path"`
	Format      Format `json:"format"`
	VirtualSize uint64 `json:"virtual_size"`
	Status      string `json:"status"`
}

type MapOptions struct {
	Path        string
	InputFormat Format
}

type MapExtent struct {
	Start  uint64 `json:"start"`
	Length uint64 `json:"length"`
	Kind   string `json:"kind"`
}

type MapResult struct {
	Format  Format      `json:"format"`
	Path    string      `json:"path"`
	Extents []MapExtent `json:"extents"`
}

type MeasureOptions struct {
	Format      Format
	Size        uint64
	ClusterBits uint32
	BlockSize   uint32
	BackingFile string
}

type MeasureResult struct {
	Format                Format `json:"format"`
	VirtualSize           uint64 `json:"virtual_size"`
	ClusterBits           uint32 `json:"cluster_bits"`
	ClusterSize           uint64 `json:"cluster_size"`
	BlockSize             uint32 `json:"block_size,omitempty"`
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
