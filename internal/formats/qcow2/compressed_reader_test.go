package qcow2

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenCompressedClusterDeflate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compressed.qcow2")

	const (
		clusterBits = 16
		clusterSize = 1 << clusterBits
		virtualSize = clusterSize
		l1Off       = clusterSize
		l2Off       = 2 * clusterSize
		compOff     = 3 * clusterSize
	)

	rawCluster := bytes.Repeat([]byte("ABCDEFGH"), clusterSize/8)

	var comp bytes.Buffer
	fw, err := flate.NewWriter(&comp, flate.DefaultCompression)
	if err != nil {
		t.Fatalf("flate writer: %v", err)
	}
	if _, err := fw.Write(rawCluster); err != nil {
		t.Fatalf("flate write: %v", err)
	}
	if err := fw.Close(); err != nil {
		t.Fatalf("flate close: %v", err)
	}

	compBytes := comp.Bytes()
	sectorsUsed := (uint64(len(compBytes)) + 511) / 512
	if sectorsUsed == 0 {
		t.Fatalf("compressed payload unexpectedly empty")
	}

	// x = 70 - cluster_bits
	x := uint32(70 - clusterBits)
	additionalSectors := sectorsUsed - 1
	compressedDesc := (uint64(1) << 62) | (additionalSectors << x) | uint64(compOff)

	// Build a minimal qcow2 image:
	// cluster 0: header
	// cluster 1: L1 table
	// cluster 2: L2 table
	// cluster 3: compressed data
	totalSize := uint64(compOff) + sectorsUsed*512
	img := make([]byte, totalSize)

	// Header
	binary.BigEndian.PutUint32(img[0:4], magicQcow2)
	binary.BigEndian.PutUint32(img[4:8], version3)
	binary.BigEndian.PutUint32(img[20:24], clusterBits)
	binary.BigEndian.PutUint64(img[24:32], virtualSize)
	binary.BigEndian.PutUint32(img[36:40], 1)     // l1_size
	binary.BigEndian.PutUint64(img[40:48], l1Off) // l1_table_offset
	binary.BigEndian.PutUint64(img[48:56], 0)     // refcount_table_offset
	binary.BigEndian.PutUint32(img[56:60], 0)     // refcount_table_clusters
	binary.BigEndian.PutUint32(img[60:64], 0)     // nb_snapshots
	binary.BigEndian.PutUint64(img[64:72], 0)     // snapshots_offset
	binary.BigEndian.PutUint64(img[72:80], 0)     // incompatible_features
	binary.BigEndian.PutUint64(img[80:88], 0)     // compatible_features
	binary.BigEndian.PutUint64(img[88:96], 0)     // autoclear_features
	binary.BigEndian.PutUint32(img[96:100], 4)    // refcount_order
	binary.BigEndian.PutUint32(img[100:104], 104) // header_length

	// L1[0] = l2Off
	binary.BigEndian.PutUint64(img[l1Off:l1Off+8], uint64(l2Off))

	// L2[0] = compressed descriptor
	binary.BigEndian.PutUint64(img[l2Off:l2Off+8], compressedDesc)

	// compressed payload
	copy(img[compOff:], compBytes)

	if err := os.WriteFile(path, img, 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	r, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer r.Close()

	got := make([]byte, len(rawCluster))
	n, err := r.ReadAt(got, 0)
	if err != nil && n != len(got) {
		t.Fatalf("read: n=%d err=%v", n, err)
	}

	if !bytes.Equal(got, rawCluster) {
		t.Fatalf("decompressed cluster mismatch")
	}
}
