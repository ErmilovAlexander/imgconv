package vmdk

import (
	"fmt"
	"math"
)

func sectorsToOffBytes(sectors uint64) (int64, error) {
	// sectors * 512 must fit into int64
	if sectors > uint64(math.MaxInt64)/sectorSize {
		return 0, fmt.Errorf("%w: sector offset overflow: %d sectors", ErrUnsupportedVMDK, sectors)
	}
	return int64(sectors * sectorSize), nil
}
