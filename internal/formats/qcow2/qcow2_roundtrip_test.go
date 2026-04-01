package qcow2

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestQCOW2RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "disk.qcow2")

	const size = 2 << 20

	w, err := Create(path, size, WriterOptions{
		ClusterBits: 16,
		Sparse:      true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	p1 := bytes.Repeat([]byte{0x11}, 8192)
	p2 := bytes.Repeat([]byte{0x22}, 4096)

	if _, err := w.WriteAt(p1, 0); err != nil {
		t.Fatalf("write p1: %v", err)
	}
	if _, err := w.WriteAt(p2, (1<<20)+123); err != nil {
		t.Fatalf("write p2: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if err := CheckFile(path); err != nil {
		t.Fatalf("check: %v", err)
	}

	r, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer r.Close()

	buf1 := make([]byte, len(p1))
	if _, err := r.ReadAt(buf1, 0); err != nil {
		t.Fatalf("read p1: %v", err)
	}
	if !bytes.Equal(buf1, p1) {
		t.Fatalf("p1 mismatch")
	}

	zeroBuf := make([]byte, 4096)
	if _, err := r.ReadAt(zeroBuf, 512<<10); err != nil {
		t.Fatalf("read zero region: %v", err)
	}
	if !bytes.Equal(zeroBuf, make([]byte, len(zeroBuf))) {
		t.Fatalf("expected zero region")
	}

	buf2 := make([]byte, len(p2))
	if _, err := r.ReadAt(buf2, (1<<20)+123); err != nil {
		t.Fatalf("read p2: %v", err)
	}
	if !bytes.Equal(buf2, p2) {
		t.Fatalf("p2 mismatch")
	}
}
