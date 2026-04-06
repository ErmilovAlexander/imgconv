package image

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectFormatByHeaderQCOW2(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "disk.bin")

	// qcow2 magic in big-endian: 0x514649fb
	data := []byte{0x51, 0x46, 0x49, 0xFB}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if got := DetectFormat(path); got != FormatQCOW2 {
		t.Fatalf("format = %q want %q", got, FormatQCOW2)
	}
}

func TestDetectFormatByHeaderVDI(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "disk.img")

	data := make([]byte, 68)
	// VDI signature at offset 64 (little-endian): 0xBEDA107F
	data[64] = 0x7F
	data[65] = 0x10
	data[66] = 0xDA
	data[67] = 0xBE
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if got := DetectFormat(path); got != FormatVDI {
		t.Fatalf("format = %q want %q", got, FormatVDI)
	}
}

func TestDetectFormatByHeaderVMDKDescriptor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "disk.bin")

	data := []byte("# Disk DescriptorFile\nversion=1\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if got := DetectFormat(path); got != FormatVMDK {
		t.Fatalf("format = %q want %q", got, FormatVMDK)
	}
}

func TestDetectFormatFallsBackToExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "disk.qcow2")

	if err := os.WriteFile(path, []byte("not-a-known-header"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if got := DetectFormat(path); got != FormatQCOW2 {
		t.Fatalf("format = %q want %q", got, FormatQCOW2)
	}
}

