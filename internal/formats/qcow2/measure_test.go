package qcow2

import "testing"

func TestMeasure(t *testing.T) {
	m, err := Measure(MeasureOptions{
		Size:        500 << 30,
		ClusterBits: 16,
	})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}

	if m.Format != "qcow2" {
		t.Fatalf("format=%q", m.Format)
	}
	if m.ClusterSize != 1<<16 {
		t.Fatalf("cluster size=%d", m.ClusterSize)
	}
	if m.MetadataBytes == 0 {
		t.Fatalf("metadata bytes should be > 0")
	}
	if m.RefcountBlockCount == 0 {
		t.Fatalf("refcount block count should be > 0")
	}
	if m.MaxDataClusters == 0 {
		t.Fatalf("max data clusters should be > 0")
	}
}
