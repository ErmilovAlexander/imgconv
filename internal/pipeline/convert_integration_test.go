package pipeline

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ErmilovAlexander/imgconv/internal/formats/qcow2"
	"github.com/ErmilovAlexander/imgconv/internal/formats/raw"
	"github.com/ErmilovAlexander/imgconv/internal/formats/vdi"
	"github.com/ErmilovAlexander/imgconv/internal/formats/vmdk"
)

func TestConvertRawToQCOW2AndVerify(t *testing.T) {
	dir := t.TempDir()

	rawPath := filepath.Join(dir, "src.raw")
	qcowPath := filepath.Join(dir, "out.qcow2")

	srcData := make([]byte, 1<<20)
	copy(srcData[0:16], []byte("hello-raw-000001"))
	copy(srcData[700000:700016], []byte("hello-raw-000002"))

	if err := os.WriteFile(rawPath, srcData, 0o644); err != nil {
		t.Fatalf("write raw: %v", err)
	}

	in, err := raw.Open(rawPath)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer in.Close()

	if err := ConvertRange(context.Background(), in, qcowPath, ConvertRangeOptions{
		Threads:   2,
		Sparse:    true,
		ChunkSize: 64 << 10,
		Format:    "qcow2",
	}); err != nil {
		t.Fatalf("convert: %v", err)
	}

	if err := VerifyRange(context.Background(), func() (RangeReader, error) {
		return raw.Open(rawPath)
	}, qcowPath, "qcow2", VerifyOptions{
		Mode:      VerifyFull,
		ChunkSize: 64 << 10,
	}); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestConvertVMDKFlatToQCOW2AndVerify(t *testing.T) {
	dir := t.TempDir()

	flatPath := filepath.Join(dir, "disk-flat.vmdk")
	descPath := filepath.Join(dir, "disk.vmdk")
	qcowPath := filepath.Join(dir, "out.qcow2")

	srcData := bytes.Repeat([]byte{0}, 1<<20)
	copy(srcData[0:16], []byte("hello-vmdk-00001"))
	copy(srcData[65536:65552], []byte("hello-vmdk-00002"))
	copy(srcData[len(srcData)-16:], []byte("hello-vmdk-tail!"))

	if err := os.WriteFile(flatPath, srcData, 0o644); err != nil {
		t.Fatalf("write flat: %v", err)
	}

	sectors := len(srcData) / 512
	desc := []byte("# Disk DescriptorFile\nversion=1\ncreateType=\"vmfs\"\n\nRW " + itoa(sectors) + " FLAT \"disk-flat.vmdk\" 0\n")
	if err := os.WriteFile(descPath, desc, 0o644); err != nil {
		t.Fatalf("write desc: %v", err)
	}

	in, err := vmdk.Open(descPath)
	if err != nil {
		t.Fatalf("open vmdk: %v", err)
	}
	defer in.Close()

	if err := ConvertRange(context.Background(), in, qcowPath, ConvertRangeOptions{
		Threads:   2,
		Sparse:    true,
		ChunkSize: 64 << 10,
		Format:    "qcow2",
	}); err != nil {
		t.Fatalf("convert: %v", err)
	}

	if err := VerifyRange(context.Background(), func() (RangeReader, error) {
		return vmdk.Open(descPath)
	}, qcowPath, "qcow2", VerifyOptions{
		Mode:      VerifyFull,
		ChunkSize: 64 << 10,
	}); err != nil {
		t.Fatalf("verify: %v", err)
	}

	r, err := qcow2.Open(qcowPath)
	if err != nil {
		t.Fatalf("open qcow2: %v", err)
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

func TestConvertRawToVDIAndVerify(t *testing.T) {
	dir := t.TempDir()

	rawPath := filepath.Join(dir, "src.raw")
	vdiPath := filepath.Join(dir, "out.vdi")

	srcData := make([]byte, 2<<20)
	copy(srcData[0:16], []byte("hello-vdi-000001"))
	copy(srcData[1500000:1500016], []byte("hello-vdi-000002"))

	if err := os.WriteFile(rawPath, srcData, 0o644); err != nil {
		t.Fatalf("write raw: %v", err)
	}

	in, err := raw.Open(rawPath)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer in.Close()

	if err := ConvertRange(context.Background(), in, vdiPath, ConvertRangeOptions{
		Threads:   2,
		Sparse:    true,
		ChunkSize: 64 << 10,
		Format:    "vdi",
	}); err != nil {
		t.Fatalf("convert: %v", err)
	}

	if err := VerifyRange(context.Background(), func() (RangeReader, error) {
		return raw.Open(rawPath)
	}, vdiPath, "vdi", VerifyOptions{
		Mode:      VerifyFull,
		ChunkSize: 64 << 10,
	}); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestConvertVDIToQCOW2AndVerify(t *testing.T) {
	dir := t.TempDir()

	vdiPath := filepath.Join(dir, "src.vdi")
	qcowPath := filepath.Join(dir, "out.qcow2")

	w, err := vdi.Create(vdiPath, 2<<20, vdi.WriterOptions{
		BlockSize: 1 << 20,
		Sparse:    true,
	})
	if err != nil {
		t.Fatalf("create vdi: %v", err)
	}
	if _, err := w.WriteAt([]byte("hello-vdi-source"), 12345); err != nil {
		t.Fatalf("write vdi: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close vdi: %v", err)
	}

	in, err := vdi.Open(vdiPath)
	if err != nil {
		t.Fatalf("open vdi: %v", err)
	}
	defer in.Close()

	if err := ConvertRange(context.Background(), in, qcowPath, ConvertRangeOptions{
		Threads:   2,
		Sparse:    true,
		ChunkSize: 64 << 10,
		Format:    "qcow2",
	}); err != nil {
		t.Fatalf("convert: %v", err)
	}

	if err := VerifyRange(context.Background(), func() (RangeReader, error) {
		return vdi.Open(vdiPath)
	}, qcowPath, "qcow2", VerifyOptions{
		Mode:      VerifyFull,
		ChunkSize: 64 << 10,
	}); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestConvertRawToQCOW2UsesClusterBits(t *testing.T) {
	dir := t.TempDir()

	rawPath := filepath.Join(dir, "src.raw")
	qcowPath := filepath.Join(dir, "out.qcow2")

	srcData := make([]byte, 1<<20)
	copy(srcData[0:16], []byte("cluster-bits-test"))
	if err := os.WriteFile(rawPath, srcData, 0o644); err != nil {
		t.Fatalf("write raw: %v", err)
	}

	in, err := raw.Open(rawPath)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer in.Close()

	if err := ConvertRange(context.Background(), in, qcowPath, ConvertRangeOptions{
		Threads:     2,
		Sparse:      true,
		ChunkSize:   64 << 10,
		Format:      "qcow2",
		ClusterBits: 17,
	}); err != nil {
		t.Fatalf("convert: %v", err)
	}

	r, err := qcow2.Open(qcowPath)
	if err != nil {
		t.Fatalf("open qcow2: %v", err)
	}
	defer r.Close()

	if got := r.ClusterBits(); got != 17 {
		t.Fatalf("cluster bits = %d want %d", got, 17)
	}
}

func TestConvertRawToVDIUsesBlockSize(t *testing.T) {
	dir := t.TempDir()

	rawPath := filepath.Join(dir, "src.raw")
	vdiPath := filepath.Join(dir, "out.vdi")

	srcData := make([]byte, 3<<20)
	copy(srcData[0:16], []byte("block-size-test1"))
	copy(srcData[2<<20:2<<20+16], []byte("block-size-test2"))
	if err := os.WriteFile(rawPath, srcData, 0o644); err != nil {
		t.Fatalf("write raw: %v", err)
	}

	in, err := raw.Open(rawPath)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer in.Close()

	if err := ConvertRange(context.Background(), in, vdiPath, ConvertRangeOptions{
		Threads:   2,
		Sparse:    true,
		ChunkSize: 64 << 10,
		Format:    "vdi",
		BlockSize: 2 << 20,
	}); err != nil {
		t.Fatalf("convert: %v", err)
	}

	r, err := vdi.Open(vdiPath)
	if err != nil {
		t.Fatalf("open vdi: %v", err)
	}
	defer r.Close()

	if got := r.BlockSize(); got != 2<<20 {
		t.Fatalf("block size = %d want %d", got, 2<<20)
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}

	var a [32]byte
	i := len(a)

	for v > 0 {
		i--
		a[i] = byte('0' + v%10)
		v /= 10
	}

	return string(a[i:])
}
