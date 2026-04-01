package qcow2

const (
	qcowOFCompressed = uint64(1) << 62
	qcowOFCopied     = uint64(1) << 63
	qcowL2Zero       = uint64(1) << 0

	qcowL2OffsetMask = ^(qcowOFCompressed | qcowOFCopied | uint64(0x1FF))
	qcowL1OffsetMask = ^(qcowOFCopied | uint64(0x1FF))
)

func entriesPerL2(clusterSize uint64) uint64 {
	return clusterSize / 8
}

func l1EntryOffset(index uint32) uint64 {
	return uint64(index) * 8
}

func l2EntryOffset(index uint64) uint64 {
	return index * 8
}
