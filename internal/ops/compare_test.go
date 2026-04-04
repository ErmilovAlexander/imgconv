package ops

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ErmilovAlexander/imgconv/internal/formats/qcow2"
	"github.com/ErmilovAlexander/imgconv/internal/pipeline"
)

func TestCompareRawAndQCOW2(t *testing.T) {
	dir := t.TempDir()

	rawPath := filepath.Join(dir, "disk.raw")
	qcowPath := filepath.Join(dir, "disk.qcow2")

	data := make([]byte, 2<<20)
	copy(data[0:16], []byte("compare-block-000"))
	copy(data[1<<20:1<<20+16], []byte("compare-block-111"))

	if err := os.WriteFile(rawPath, data, 0o644); err != nil {
		t.Fatalf("write raw: %v", err)
	}

	w, err := qcow2.Create(qcowPath, uint64(len(data)), qcow2.WriterOptions{
		ClusterBits: 16,
		Sparse:      true,
	})
	if err != nil {
		t.Fatalf("create qcow2: %v", err)
	}
	if _, err := w.WriteAt(data, 0); err != nil {
		t.Fatalf("write qcow2: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close qcow2: %v", err)
	}

	if err := ComparePaths(context.Background(), rawPath, "raw", qcowPath, "qcow2", CompareOptions{
		Mode:      pipeline.VerifyFull,
		ChunkSize: 64 << 10,
	}); err != nil {
		t.Fatalf("compare failed: %v", err)
	}
}
