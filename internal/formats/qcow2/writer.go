package qcow2

import (
	"encoding/binary"
	"fmt"
	"os"
)

type WriterOptions struct {
	ClusterBits uint32
	Sparse      bool
	BackingFile string
}

type Writer struct {
	f           *os.File
	h           Header
	clusterSize uint64

	l1      []uint64
	l2cache map[uint32][]uint64

	sparse  bool
	closed  bool
	nextOff uint64

	backingFile string

	refcountTableClusters uint64
	refcountBlockCount    uint64
	refcountBlockOffsets  []uint64
	l1Clusters            uint64
}

func Create(path string, size uint64, opts WriterOptions) (*Writer, error) {
	if size == 0 {
		return nil, fmt.Errorf("qcow2: zero size")
	}

	clusterBits := opts.ClusterBits
	if clusterBits == 0 {
		clusterBits = 16 // 64 KiB
	}
	clusterSize := uint64(1) << clusterBits
	if clusterSize < 4096 {
		return nil, fmt.Errorf("qcow2: cluster size too small")
	}

	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}

	epl2 := entriesPerL2(clusterSize)
	l1Size := uint32((size + clusterSize*epl2 - 1) / (clusterSize * epl2))
	if l1Size == 0 {
		l1Size = 1
	}

	l1Bytes := uint64(l1Size) * 8
	l1Clusters := ceilDiv(l1Bytes, clusterSize)
	if l1Clusters == 0 {
		l1Clusters = 1
	}

	dataClustersMax := ceilDiv(size, clusterSize)
	l2ClustersMax := ceilDiv(dataClustersMax, epl2)

	refcountBlockEntries := clusterSize / 2
	if refcountBlockEntries == 0 {
		_ = f.Close()
		return nil, fmt.Errorf("qcow2: invalid refcount block geometry")
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
			l2ClustersMax

		totalClustersMax := metadataClusters + dataClustersMax
		newRefcountBlockCount := ceilDiv(totalClustersMax, refcountBlockEntries)
		if newRefcountBlockCount == 0 {
			newRefcountBlockCount = 1
		}
		if newRefcountBlockCount == refcountBlockCount {
			break
		}
		refcountBlockCount = newRefcountBlockCount
	}

	h := Header{
		Magic:                 magicQcow2,
		Version:               version3,
		ClusterBits:           clusterBits,
		Size:                  size,
		L1Size:                l1Size,
		RefcountOrder:         4,
		HeaderLength:          104,
		CompressionType:       compressionDeflate,
		RefcountTableClusters: uint32(refcountTableClusters),
		RefcountClusters:      uint32(refcountTableClusters),
	}

	// backing filename lives in the first cluster, right after the header.
	if opts.BackingFile != "" {
		backingBytes := []byte(opts.BackingFile)
		if uint64(h.HeaderLength)+uint64(len(backingBytes)) > clusterSize {
			_ = f.Close()
			return nil, fmt.Errorf("qcow2: backing file name does not fit into header cluster")
		}
		h.BackingFileOffset = uint64(h.HeaderLength)
		h.BackingFileSize = uint32(len(backingBytes))
	}

	// Layout:
	// cluster 0                               : header + optional backing filename
	// cluster 1 .. 1+refcountTableClusters-1  : refcount table
	// next refcountBlockCount clusters        : refcount blocks
	// next l1Clusters clusters                : L1 table
	// rest                                    : L2 tables + data clusters
	h.RefcountTableOffset = clusterSize
	h.RefcountOffset = h.RefcountTableOffset

	refcountBlocksStart := uint64(1) + refcountTableClusters
	l1Start := refcountBlocksStart + refcountBlockCount
	h.L1TableOffset = l1Start * clusterSize

	nextOff := (l1Start + l1Clusters) * clusterSize

	refcountBlockOffsets := make([]uint64, refcountBlockCount)
	for i := uint64(0); i < refcountBlockCount; i++ {
		refcountBlockOffsets[i] = (refcountBlocksStart + i) * clusterSize
	}

	l1 := make([]uint64, h.L1Size)

	w := &Writer{
		f:                     f,
		h:                     h,
		clusterSize:           clusterSize,
		l1:                    l1,
		l2cache:               make(map[uint32][]uint64),
		sparse:                opts.Sparse,
		nextOff:               nextOff,
		backingFile:           opts.BackingFile,
		refcountTableClusters: refcountTableClusters,
		refcountBlockCount:    refcountBlockCount,
		refcountBlockOffsets:  refcountBlockOffsets,
		l1Clusters:            l1Clusters,
	}

	if err := f.Truncate(int64(nextOff)); err != nil {
		_ = f.Close()
		return nil, err
	}

	if err := w.writeHeader(); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := w.writeBackingFileName(); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := w.writeRefcountTable(); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := w.zeroRefcountBlocks(); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := w.writeL1(); err != nil {
		_ = f.Close()
		return nil, err
	}

	if err := w.markInitialMetadataRefcounts(); err != nil {
		_ = f.Close()
		return nil, err
	}

	return w, nil
}

func (w *Writer) Size() uint64 {
	return w.h.Size
}

func (w *Writer) WriteAt(p []byte, off int64) (int, error) {
	if w.closed {
		return 0, fmt.Errorf("qcow2: write on closed writer")
	}
	if off < 0 {
		return 0, fmt.Errorf("qcow2: negative offset")
	}
	if uint64(off) >= w.h.Size {
		return 0, nil
	}

	max := w.h.Size - uint64(off)
	if uint64(len(p)) > max {
		p = p[:max]
	}

	written := 0
	for written < len(p) {
		curOff := uint64(off) + uint64(written)
		clusterIdx := curOff / w.clusterSize
		inCluster := curOff % w.clusterSize

		want := uint64(len(p) - written)
		if want > w.clusterSize-inCluster {
			want = w.clusterSize - inCluster
		}

		chunk := p[written : written+int(want)]

		if w.sparse && inCluster == 0 && want == w.clusterSize && isAllZero(chunk) {
			written += int(want)
			continue
		}

		dataOff, err := w.ensureDataCluster(clusterIdx)
		if err != nil {
			return written, err
		}

		n, err := w.f.WriteAt(chunk, int64(dataOff+inCluster))
		written += n
		if err != nil {
			return written, err
		}
	}

	return written, nil
}

func (w *Writer) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true

	if err := w.flushAllL2(); err != nil {
		_ = w.f.Close()
		return err
	}
	if err := w.writeL1(); err != nil {
		_ = w.f.Close()
		return err
	}
	if err := w.writeHeader(); err != nil {
		_ = w.f.Close()
		return err
	}
	if err := w.writeBackingFileName(); err != nil {
		_ = w.f.Close()
		return err
	}

	return w.f.Close()
}

func (w *Writer) writeHeader() error {
	return w.h.WriteAt(w.f, 0)
}

func (w *Writer) writeBackingFileName() error {
	if w.backingFile == "" || w.h.BackingFileOffset == 0 || w.h.BackingFileSize == 0 {
		return nil
	}
	_, err := w.f.WriteAt([]byte(w.backingFile), int64(w.h.BackingFileOffset))
	return err
}

func (w *Writer) writeRefcountTable() error {
	bufSize := w.refcountTableClusters * w.clusterSize
	buf := make([]byte, bufSize)

	for i, off := range w.refcountBlockOffsets {
		base := i * 8
		binary.BigEndian.PutUint64(buf[base:base+8], off)
	}

	_, err := w.f.WriteAt(buf, int64(w.h.RefcountTableOffset))
	return err
}

func (w *Writer) zeroRefcountBlocks() error {
	zero := make([]byte, w.clusterSize)
	for _, off := range w.refcountBlockOffsets {
		if _, err := w.f.WriteAt(zero, int64(off)); err != nil {
			return err
		}
	}
	return nil
}

func (w *Writer) writeZeroCluster(off uint64) error {
	buf := make([]byte, w.clusterSize)
	_, err := w.f.WriteAt(buf, int64(off))
	return err
}

func (w *Writer) writeL1() error {
	bufSize := w.l1Clusters * w.clusterSize
	buf := make([]byte, bufSize)

	for i, v := range w.l1 {
		base := i * 8
		binary.BigEndian.PutUint64(buf[base:base+8], v)
	}

	_, err := w.f.WriteAt(buf, int64(w.h.L1TableOffset))
	return err
}

func (w *Writer) markInitialMetadataRefcounts() error {
	if err := w.setRefcount(0, 1); err != nil {
		return err
	}

	for i := uint64(0); i < w.refcountTableClusters; i++ {
		if err := w.setRefcount((uint64(1)+i)*w.clusterSize, 1); err != nil {
			return err
		}
	}

	for _, off := range w.refcountBlockOffsets {
		if err := w.setRefcount(off, 1); err != nil {
			return err
		}
	}

	l1StartCluster := w.h.L1TableOffset / w.clusterSize
	for i := uint64(0); i < w.l1Clusters; i++ {
		if err := w.setRefcount((l1StartCluster+i)*w.clusterSize, 1); err != nil {
			return err
		}
	}

	return nil
}

func (w *Writer) flushAllL2() error {
	for l1Idx, table := range w.l2cache {
		if err := w.flushL2(uint32(l1Idx), table); err != nil {
			return err
		}
	}
	return nil
}

func (w *Writer) flushL2(l1Idx uint32, table []uint64) error {
	if w.l1[l1Idx] == 0 {
		return fmt.Errorf("qcow2: flush l2 without allocated l1 entry")
	}

	l2Off := w.l1[l1Idx] & qcowL1OffsetMask

	buf := make([]byte, w.clusterSize)
	for i, v := range table {
		base := i * 8
		binary.BigEndian.PutUint64(buf[base:base+8], v)
	}
	_, err := w.f.WriteAt(buf, int64(l2Off))
	return err
}

func (w *Writer) ensureL2(l1Idx uint32) ([]uint64, error) {
	if table, ok := w.l2cache[l1Idx]; ok {
		return table, nil
	}

	if w.l1[l1Idx] == 0 {
		off, err := w.allocCluster()
		if err != nil {
			return nil, err
		}
		if err := w.setRefcount(off, 1); err != nil {
			return nil, err
		}
		if err := w.writeZeroCluster(off); err != nil {
			return nil, err
		}

		w.l1[l1Idx] = off | qcowOFCopied
	}

	table := make([]uint64, entriesPerL2(w.clusterSize))
	w.l2cache[l1Idx] = table
	return table, nil
}

func (w *Writer) ensureDataCluster(clusterIdx uint64) (uint64, error) {
	epl2 := entriesPerL2(w.clusterSize)
	l1Idx := uint32(clusterIdx / epl2)
	l2Idx := uint32(clusterIdx % epl2)

	table, err := w.ensureL2(l1Idx)
	if err != nil {
		return 0, err
	}

	if table[l2Idx] != 0 {
		return table[l2Idx] & qcowL2OffsetMask, nil
	}

	off, err := w.allocCluster()
	if err != nil {
		return 0, err
	}
	if err := w.setRefcount(off, 1); err != nil {
		return 0, err
	}
	if err := w.writeZeroCluster(off); err != nil {
		return 0, err
	}

	table[l2Idx] = off | qcowOFCopied
	if err := w.flushL2(l1Idx, table); err != nil {
		return 0, err
	}

	return off, nil
}

func (w *Writer) allocCluster() (uint64, error) {
	off := w.nextOff
	w.nextOff += w.clusterSize
	if err := w.f.Truncate(int64(w.nextOff)); err != nil {
		return 0, err
	}
	return off, nil
}

func (w *Writer) setRefcount(clusterOff uint64, value uint16) error {
	clusterIndex := clusterOff / w.clusterSize
	refcountBlockEntries := w.clusterSize / 2

	tableIdx := clusterIndex / refcountBlockEntries
	blockIdx := clusterIndex % refcountBlockEntries

	if tableIdx >= uint64(len(w.refcountBlockOffsets)) {
		return fmt.Errorf("qcow2: refcount table index out of range: cluster_index=%d table_idx=%d max=%d", clusterIndex, tableIdx, len(w.refcountBlockOffsets))
	}

	refcountBlockOff := w.refcountBlockOffsets[tableIdx]
	entryOff := refcountBlockOff + blockIdx*2

	var raw [2]byte
	binary.BigEndian.PutUint16(raw[:], value)
	_, err := w.f.WriteAt(raw[:], int64(entryOff))
	return err
}

func isAllZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

func ceilDiv(x, y uint64) uint64 {
	if y == 0 {
		return 0
	}
	return (x + y - 1) / y
}
