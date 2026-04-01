package vdi

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestVDIRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "disk.vdi")

	w, err := Create(path, 3<<20, WriterOptions{
		BlockSize: 1 << 20,
		Sparse:    true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	p1 := bytes.Repeat([]byte{0x41}, 8192)
	p2 := bytes.Repeat([]byte{0x52}, 4096)

	if _, err := w.WriteAt(p1, 0); err != nil {
		t.Fatalf("write p1: %v", err)
	}
	if _, err := w.WriteAt(p2, (2<<20)+123); err != nil {
		t.Fatalf("write p2: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	r, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer r.Close()

	buf1 := make([]byte, len(p1))
	n, err := r.ReadAt(buf1, 0)
	if err != nil && n != len(buf1) {
		t.Fatalf("read p1: n=%d err=%v", n, err)
	}
	if !bytes.Equal(buf1, p1) {
		t.Fatalf("p1 mismatch")
	}

	zeroBuf := make([]byte, 4096)
	n, err = r.ReadAt(zeroBuf, 1<<20)
	if err != nil && n != len(zeroBuf) {
		t.Fatalf("read zero region: n=%d err=%v", n, err)
	}
	if !bytes.Equal(zeroBuf, make([]byte, len(zeroBuf))) {
		t.Fatalf("expected zero region")
	}

	buf2 := make([]byte, len(p2))
	n, err = r.ReadAt(buf2, (2<<20)+123)
	if err != nil && n != len(buf2) {
		t.Fatalf("read p2: n=%d err=%v", n, err)
	}
	if !bytes.Equal(buf2, p2) {
		t.Fatalf("p2 mismatch")
	}
}
