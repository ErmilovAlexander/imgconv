package vmdk

import (
	"encoding/hex"
	"fmt"
)

func countNonZeroU64(a []uint64) int {
	n := 0
	for _, v := range a {
		if v != 0 {
			n++
		}
	}
	return n
}

func countEventTypes(events []streamEvent) map[uint32]int {
	m := make(map[uint32]int)
	for _, e := range events {
		m[e.Type]++
	}
	return m
}

func markerName(t uint32) string {
	switch t {
	case streamMarkerEOS:
		return "EOS"
	case streamMarkerGT:
		return "GT"
	case streamMarkerGD:
		return "GD"
	case streamMarkerFOOTER:
		return "FOOTER"
	case streamEventDATA:
		return "DATA"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", t)
	}
}

func hexdumpPrefix(b []byte, n int) string {
	if n > len(b) {
		n = len(b)
	}
	return hex.Dump(b[:n])
}
