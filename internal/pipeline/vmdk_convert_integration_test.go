package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"imgconv/internal/formats/raw"
	"imgconv/internal/formats/vmdk"
)

func TestConvertRawToVMDKAndVerify(t *testing.T) {
	dir := t.TempDir()

	rawPath := filepath.Join(dir, "src.raw")
	vmdkPath := filepath.Join(dir, "out.vmdk")

	srcData := make([]byte, 2<<20)
	copy(srcData[0:16], []byte("hello-vmdk-00001"))
	copy(srcData[1500000:1500016], []byte("hello-vmdk-00002"))
	copy(srcData[len(srcData)-16:], []byte("hello-vmdk-tail!"))

	if err := os.WriteFile(rawPath, srcData, 0o644); err != nil {
		t.Fatalf("write raw: %v", err)
	}

	in, err := raw.Open(rawPath)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer in.Close()

	if err := ConvertRange(context.Background(), in, vmdkPath, ConvertRangeOptions{
		Threads:   2,
		Sparse:    true,
		ChunkSize: 64 << 10,
		Format:    "vmdk",
	}); err != nil {
		t.Fatalf("convert: %v", err)
	}

	if err := VerifyRange(context.Background(), func() (RangeReader, error) {
		return raw.Open(rawPath)
	}, vmdkPath, "vmdk", VerifyOptions{
		Mode:      VerifyFull,
		ChunkSize: 64 << 10,
	}); err != nil {
		t.Fatalf("verify: %v", err)
	}

	r, err := vmdk.Open(vmdkPath)
	if err != nil {
		t.Fatalf("open vmdk: %v", err)
	}
	defer r.Close()

	buf := make([]byte, 16)
	if _, err := r.ReadAt(buf, int64(len(srcData)-16)); err != nil {
		t.Fatalf("read tail: %v", err)
	}
	if string(buf) != "hello-vmdk-tail!" {
		t.Fatalf("tail mismatch: %q", string(buf))
	}
}
