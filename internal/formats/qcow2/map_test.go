package qcow2

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestMapFileBasic(t *testing.T) {
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
	if _, err := baseW.WriteAt(bytes.Repeat([]byte{0x41}, 4096), 0); err != nil {
		t.Fatalf("write base: %v", err)
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
	if _, err := overlayW.WriteAt(bytes.Repeat([]byte{0x55}, 4096), 2<<20); err != nil {
		t.Fatalf("write overlay: %v", err)
	}
	if err := overlayW.Close(); err != nil {
		t.Fatalf("close overlay: %v", err)
	}

	exts, err := MapFile(overlayPath)
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if len(exts) == 0 {
		t.Fatalf("expected non-empty map")
	}

	var seenBacking, seenData bool
	for _, e := range exts {
		if e.Kind == MapKindBacking {
			seenBacking = true
		}
		if e.Kind == MapKindData {
			seenData = true
		}
	}
	if !seenBacking {
		t.Fatalf("expected backing extents")
	}
	if !seenData {
		t.Fatalf("expected data extents")
	}
}
