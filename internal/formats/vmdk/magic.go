package vmdk

import (
	"encoding/binary"
	"os"
)

const (
	// Sparse extent header magic (little-endian uint32)
	// 0x564D444B == "KDMV" in file byte order
	vmdkSparseMagicLE = 0x564D444B
)

func hasSparseMagic(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	buf := make([]byte, 4)
	n, err := f.Read(buf)
	if err != nil {
		return false, err
	}
	if n < 4 {
		return false, nil
	}

	return binary.LittleEndian.Uint32(buf) == vmdkSparseMagicLE, nil
}
