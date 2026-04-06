package pipeline

import (
	"fmt"

	"github.com/ErmilovAlexander/imgconv/internal/formats/qcow2"
	"github.com/ErmilovAlexander/imgconv/internal/formats/vdi"
	"github.com/ErmilovAlexander/imgconv/internal/formats/vmdk"
)

func CheckQCOW2(path string) error {
	return qcow2.CheckFile(path)
}

func CheckVDI(path string) error {
	r, err := vdi.Open(path)
	if err != nil {
		return err
	}
	defer r.Close()

	if r.Size() == 0 {
		return fmt.Errorf("vdi: zero virtual size")
	}
	if r.BlockSize() == 0 {
		return fmt.Errorf("vdi: zero block size")
	}
	return nil
}

func CheckVMDK(path string) error {
	r, err := vmdk.Open(path)
	if err != nil {
		return err
	}
	defer r.Close()

	size := r.Size()
	if size == 0 {
		return fmt.Errorf("vmdk: zero virtual size")
	}

	buf := make([]byte, 1)
	if _, err := r.ReadAt(buf, 0); err != nil {
		return fmt.Errorf("vmdk: read at offset 0 failed: %w", err)
	}
	if _, err := r.ReadAt(buf, int64(size-1)); err != nil {
		return fmt.Errorf("vmdk: read at end failed: %w", err)
	}
	return nil
}
