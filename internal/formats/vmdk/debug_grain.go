package vmdk

import (
	"os"
	"strconv"
)

func debugGrainTarget() (uint64, bool) {
	s := os.Getenv("IMGCON_DEBUG_GRAIN")
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
