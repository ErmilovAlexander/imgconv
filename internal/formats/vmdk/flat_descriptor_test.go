package vmdk

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenFlatDescriptor(t *testing.T) {
	dir := t.TempDir()

	flatPath := filepath.Join(dir, "disk-flat.vmdk")
	descPath := filepath.Join(dir, "disk.vmdk")

	content := make([]byte, 4096)
	copy(content[0:8], []byte("ABCDEFGH"))
	copy(content[1024:1032], []byte("IJKLMNOP"))

	if err := os.WriteFile(flatPath, content, 0o644); err != nil {
		t.Fatalf("write flat: %v", err)
	}

	desc := `# Disk DescriptorFile
version=1
encoding="UTF-8"
CID=fffffffe
parentCID=ffffffff
createType="vmfs"

RW 8 FLAT "disk-flat.vmdk" 0
`
	if err := os.WriteFile(descPath, []byte(desc), 0o644); err != nil {
		t.Fatalf("write desc: %v", err)
	}

	r, err := Open(descPath)
	if err != nil {
		t.Fatalf("open descriptor: %v", err)
	}
	defer r.Close()

	if got, want := r.Size(), uint64(len(content)); got != want {
		t.Fatalf("size=%d want=%d", got, want)
	}

	buf := make([]byte, len(content))
	n, err := r.ReadAt(buf, 0)
	if err != nil && n != len(content) {
		t.Fatalf("read: n=%d err=%v", n, err)
	}

	if !bytes.Equal(buf[:len(content)], content) {
		t.Fatalf("content mismatch")
	}
}
