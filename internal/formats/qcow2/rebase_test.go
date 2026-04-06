package qcow2

import (
	"path/filepath"
	"testing"
)

func TestRebasePath(t *testing.T) {
	dir := t.TempDir()

	basePath := filepath.Join(dir, "base.qcow2")
	otherBasePath := filepath.Join(dir, "other-base.qcow2")
	overlayPath := filepath.Join(dir, "overlay.qcow2")

	baseW, err := Create(basePath, 8<<20, WriterOptions{
		ClusterBits: 16,
		Sparse:      true,
	})
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	if err := baseW.Close(); err != nil {
		t.Fatalf("close base: %v", err)
	}

	otherBaseW, err := Create(otherBasePath, 8<<20, WriterOptions{
		ClusterBits: 16,
		Sparse:      true,
	})
	if err != nil {
		t.Fatalf("create other base: %v", err)
	}
	if err := otherBaseW.Close(); err != nil {
		t.Fatalf("close other base: %v", err)
	}

	overlayW, err := Create(overlayPath, 8<<20, WriterOptions{
		ClusterBits: 16,
		Sparse:      true,
		BackingFile: "base.qcow2",
	})
	if err != nil {
		t.Fatalf("create overlay: %v", err)
	}
	if err := overlayW.Close(); err != nil {
		t.Fatalf("close overlay: %v", err)
	}

	if err := RebasePath(overlayPath, "other-base.qcow2"); err != nil {
		t.Fatalf("rebase path: %v", err)
	}

	r, err := Open(overlayPath)
	if err != nil {
		t.Fatalf("open rebased overlay: %v", err)
	}
	defer r.Close()

	if got := r.BackingFile(); got != "other-base.qcow2" {
		t.Fatalf("backing file = %q want %q", got, "other-base.qcow2")
	}

	if err := RebasePath(overlayPath, ""); err != nil {
		t.Fatalf("clear backing path: %v", err)
	}

	r2, err := Open(overlayPath)
	if err != nil {
		t.Fatalf("open overlay after clear: %v", err)
	}
	defer r2.Close()

	if got := r2.BackingFile(); got != "" {
		t.Fatalf("backing file after clear = %q want empty", got)
	}
}
