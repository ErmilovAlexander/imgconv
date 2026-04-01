package qcow2

import (
	"fmt"
	"os"
)

func RebasePath(path string, newBacking string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	h, err := readHeaderAt(f)
	if err != nil {
		return err
	}

	clusterSize := uint64(1) << h.ClusterBits
	if clusterSize == 0 {
		return fmt.Errorf("qcow2: invalid cluster size")
	}

	writeOff := uint64(h.HeaderLength)
	maxBytes := clusterSize - writeOff

	if newBacking == "" {
		// clear backing reference
		if h.BackingFileOffset != 0 && h.BackingFileSize != 0 {
			zero := make([]byte, h.BackingFileSize)
			if _, err := f.WriteAt(zero, int64(h.BackingFileOffset)); err != nil {
				return err
			}
		}
		h.BackingFileOffset = 0
		h.BackingFileSize = 0
		return h.WriteAt(f, 0)
	}

	if uint64(len([]byte(newBacking))) > maxBytes {
		return fmt.Errorf("qcow2: backing file name does not fit into header cluster")
	}

	// zero the whole name area inside first cluster after header for cleanliness
	zero := make([]byte, maxBytes)
	if _, err := f.WriteAt(zero, int64(writeOff)); err != nil {
		return err
	}

	if _, err := f.WriteAt([]byte(newBacking), int64(writeOff)); err != nil {
		return err
	}

	h.BackingFileOffset = writeOff
	h.BackingFileSize = uint32(len([]byte(newBacking)))
	return h.WriteAt(f, 0)
}
