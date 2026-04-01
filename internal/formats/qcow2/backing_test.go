package qcow2

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestQCOW2BackingReadChain(t *testing.T) {
	dir := t.TempDir()

	basePath := filepath.Join(dir, "base.qcow2")
	overlayPath := filepath.Join(dir, "overlay.qcow2")

	baseW, err := Create(basePath, 8<<20, WriterOptions{
		ClusterBits: 16,
		Sparse:      true,
	})
	if err != nil {
		t.Fatalf("create base: %v", err)
	}

	baseA := bytes.Repeat([]byte{0x41}, 4096)
	baseB := bytes.Repeat([]byte{0x42}, 4096)

	if _, err := baseW.WriteAt(baseA, 0); err != nil {
		t.Fatalf("write baseA: %v", err)
	}
	if _, err := baseW.WriteAt(baseB, 2<<20); err != nil {
		t.Fatalf("write baseB: %v", err)
	}
	if err := baseW.Close(); err != nil {
		t.Fatalf("close base: %v", err)
	}

	overlayW, err := Create(overlayPath, 8<<20, WriterOptions{
		ClusterBits: 16,
		Sparse:      true,
		BackingFile: "base.qcow2",
	})
	if err != nil {
		t.Fatalf("create overlay: %v", err)
	}

	override := bytes.Repeat([]byte{0x55}, 4096)
	if _, err := overlayW.WriteAt(override, 4<<20); err != nil {
		t.Fatalf("write override: %v", err)
	}
	if err := overlayW.Close(); err != nil {
		t.Fatalf("close overlay: %v", err)
	}

	r, err := Open(overlayPath)
	if err != nil {
		t.Fatalf("open overlay: %v", err)
	}
	defer r.Close()

	if got := r.BackingFile(); got != "base.qcow2" {
		t.Fatalf("backing file = %q want %q", got, "base.qcow2")
	}

	buf := make([]byte, 4096)

	if _, err := r.ReadAt(buf, 0); err != nil {
		t.Fatalf("read from backing region A: %v", err)
	}
	if !bytes.Equal(buf, baseA) {
		t.Fatalf("backing region A mismatch")
	}

	if _, err := r.ReadAt(buf, 2<<20); err != nil {
		t.Fatalf("read from backing region B: %v", err)
	}
	if !bytes.Equal(buf, baseB) {
		t.Fatalf("backing region B mismatch")
	}

	if _, err := r.ReadAt(buf, 4<<20); err != nil {
		t.Fatalf("read override region: %v", err)
	}
	if !bytes.Equal(buf, override) {
		t.Fatalf("override region mismatch")
	}
}
