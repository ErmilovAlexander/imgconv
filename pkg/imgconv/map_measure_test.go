package imgconv

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ErmilovAlexander/imgconv/internal/formats/vmdk"
)

func TestMapRawSingleExtent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "disk.raw")
	data := make([]byte, 1<<20)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write raw: %v", err)
	}

	res, err := Map(MapOptions{Path: path, InputFormat: FormatAuto})
	if err != nil {
		t.Fatalf("map raw: %v", err)
	}
	if res.Format != FormatRAW {
		t.Fatalf("format = %q want %q", res.Format, FormatRAW)
	}
	if len(res.Extents) != 1 {
		t.Fatalf("extents count = %d want 1", len(res.Extents))
	}
	if res.Extents[0].Start != 0 || res.Extents[0].Length != uint64(len(data)) || res.Extents[0].Kind != "data" {
		t.Fatalf("unexpected extent: %+v", res.Extents[0])
	}
}

func TestMeasureRawAndVDI(t *testing.T) {
	rawRes, err := Measure(MeasureOptions{
		Format: FormatRAW,
		Size:   3 << 20,
	})
	if err != nil {
		t.Fatalf("measure raw: %v", err)
	}
	if rawRes.MetadataBytes != 0 {
		t.Fatalf("raw metadata_bytes = %d want 0", rawRes.MetadataBytes)
	}

	vdiRes, err := Measure(MeasureOptions{
		Format:    FormatVDI,
		Size:      3 << 20,
		BlockSize: 2 << 20,
	})
	if err != nil {
		t.Fatalf("measure vdi: %v", err)
	}
	if vdiRes.BlockSize != 2<<20 {
		t.Fatalf("vdi block_size = %d want %d", vdiRes.BlockSize, 2<<20)
	}
	// blocks=2, map bytes=8, 512+8=520 aligned to 4096.
	if vdiRes.MetadataBytes != 4096 {
		t.Fatalf("vdi metadata_bytes = %d want 4096", vdiRes.MetadataBytes)
	}
}

func TestCheckVMDK(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "disk.vmdk")

	w, err := vmdk.Create(path, 2<<20, vmdk.WriterOptions{Sparse: true})
	if err != nil {
		t.Fatalf("create vmdk: %v", err)
	}
	if _, err := w.WriteAt([]byte("hello-vmdk"), 12345); err != nil {
		t.Fatalf("write vmdk: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close vmdk: %v", err)
	}

	res, err := Check(path, CheckOptions{})
	if err != nil {
		t.Fatalf("check vmdk: %v", err)
	}
	if res.Format != FormatVMDK || res.Status != "OK" {
		t.Fatalf("unexpected check result: %+v", res)
	}
}

