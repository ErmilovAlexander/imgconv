package ops

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ErmilovAlexander/imgconv/internal/formats/qcow2"
)

func TestCommitQCOW2Overlay(t *testing.T) {
	dir := t.TempDir()

	basePath := filepath.Join(dir, "base.qcow2")
	overlayPath := filepath.Join(dir, "overlay.qcow2")

	baseW, err := qcow2.Create(basePath, 8<<20, qcow2.WriterOptions{
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

	overlayW, err := qcow2.Create(overlayPath, 8<<20, qcow2.WriterOptions{
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

	if err := CommitQCOW2Overlay(context.Background(), overlayPath, CommitOptions{
		ChunkSize: 64 << 10,
		Sparse:    true,
	}); err != nil {
		t.Fatalf("commit overlay: %v", err)
	}

	baseR, err := qcow2.Open(basePath)
	if err != nil {
		t.Fatalf("open committed base: %v", err)
	}
	defer baseR.Close()

	buf := make([]byte, 4096)
	if _, err := baseR.ReadAt(buf, 0); err != nil {
		t.Fatalf("read base region A: %v", err)
	}
	if !bytes.Equal(buf, baseA) {
		t.Fatalf("base region A mismatch after commit")
	}

	if _, err := baseR.ReadAt(buf, 2<<20); err != nil {
		t.Fatalf("read base region B: %v", err)
	}
	if !bytes.Equal(buf, baseB) {
		t.Fatalf("base region B mismatch after commit")
	}

	if _, err := baseR.ReadAt(buf, 4<<20); err != nil {
		t.Fatalf("read committed override region: %v", err)
	}
	if !bytes.Equal(buf, override) {
		t.Fatalf("override not committed into base")
	}

	overlayR, err := qcow2.Open(overlayPath)
	if err != nil {
		t.Fatalf("open reset overlay: %v", err)
	}
	defer overlayR.Close()

	if got := overlayR.BackingFile(); got != "base.qcow2" {
		t.Fatalf("overlay backing file after commit = %q", got)
	}

	if _, err := overlayR.ReadAt(buf, 4<<20); err != nil {
		t.Fatalf("read overlay after reset: %v", err)
	}
	if !bytes.Equal(buf, override) {
		t.Fatalf("overlay chain read mismatch after commit/reset")
	}

	if _, err := os.Stat(overlayPath + ".imgconv-commit-inprogress"); !os.IsNotExist(err) {
		t.Fatalf("commit marker should be removed, stat err=%v", err)
	}
}

func TestRecoverCommitState(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "base.qcow2")
	overlayPath := filepath.Join(dir, "overlay.qcow2")

	baseW, err := qcow2.Create(basePath, 8<<20, qcow2.WriterOptions{
		ClusterBits: 16,
		Sparse:      true,
	})
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	if err := baseW.Close(); err != nil {
		t.Fatalf("close base: %v", err)
	}

	overlayW, err := qcow2.Create(overlayPath, 8<<20, qcow2.WriterOptions{
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

	marker := overlayPath + ".imgconv-commit-inprogress"
	if err := os.WriteFile(marker, []byte("stale"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	staleTmpOverlay := overlayPath + ".imgconv-reset-tmp"
	if err := os.WriteFile(staleTmpOverlay, []byte("tmp"), 0o644); err != nil {
		t.Fatalf("write tmp overlay: %v", err)
	}
	staleTmpBacking := basePath + ".imgconv-commit-tmp"
	if err := os.WriteFile(staleTmpBacking, []byte("tmp"), 0o644); err != nil {
		t.Fatalf("write tmp backing: %v", err)
	}

	if err := RecoverCommitState(overlayPath); err != nil {
		t.Fatalf("recover commit state: %v", err)
	}

	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("marker not removed, err=%v", err)
	}
	if _, err := os.Stat(staleTmpOverlay); !os.IsNotExist(err) {
		t.Fatalf("tmp overlay not removed, err=%v", err)
	}
	if _, err := os.Stat(staleTmpBacking); !os.IsNotExist(err) {
		t.Fatalf("tmp backing not removed, err=%v", err)
	}
}
