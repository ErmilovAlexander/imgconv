package pipeline

import (
	"fmt"

	"imgconv/internal/formats/qcow2"
	"imgconv/internal/formats/vdi"
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
