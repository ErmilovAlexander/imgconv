package qcow2

import (
	"encoding/binary"
	"fmt"
	"os"
)

func CheckFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return err
	}
	fileSize := uint64(st.Size())

	r, err := Open(path)
	if err != nil {
		return err
	}
	defer r.Close()

	clusterSize := uint64(1) << r.h.ClusterBits
	if clusterSize == 0 {
		return fmt.Errorf("qcow2: zero cluster size")
	}

	if r.h.L1Size == 0 {
		return fmt.Errorf("qcow2: zero l1 size")
	}
	if r.h.L1TableOffset == 0 {
		return fmt.Errorf("qcow2: zero l1 table offset")
	}
	if r.h.L1TableOffset >= fileSize {
		return fmt.Errorf("qcow2: l1 table offset past EOF")
	}

	l1Bytes := uint64(r.h.L1Size) * 8
	if r.h.L1TableOffset+l1Bytes > fileSize {
		return fmt.Errorf("qcow2: l1 table exceeds EOF")
	}

	entries := entriesPerL2(clusterSize)
	if entries == 0 {
		return fmt.Errorf("qcow2: bad entries per l2")
	}

	l2buf := make([]byte, clusterSize)

	for l1i, l1e := range r.l1 {
		if l1e == 0 {
			continue
		}

		l2Off := l1e & qcowL1OffsetMask
		if l2Off == 0 {
			return fmt.Errorf("qcow2: zero l2 offset: l1=%d", l1i)
		}
		if l2Off >= fileSize {
			return fmt.Errorf("qcow2: l2 off past EOF: l1=%d off=%d", l1i, l2Off)
		}
		if l2Off+clusterSize > fileSize {
			return fmt.Errorf("qcow2: l2 table exceeds EOF: l1=%d off=%d", l1i, l2Off)
		}
		if l2Off%clusterSize != 0 {
			return fmt.Errorf("qcow2: unaligned l2 table offset: l1=%d off=%d", l1i, l2Off)
		}

		if _, err := f.ReadAt(l2buf, int64(l2Off)); err != nil {
			return fmt.Errorf("qcow2: read l2 table l1=%d: %w", l1i, err)
		}

		for l2i := uint64(0); l2i < entries; l2i++ {
			l2e := binary.BigEndian.Uint64(l2buf[l2i*8 : l2i*8+8])
			if l2e == 0 {
				continue
			}

			if (l2e & qcowOFCompressed) != 0 {
				hostOff, compBytes, err := decodeCompressedDescriptor(l2e, r.h.ClusterBits)
				if err != nil {
					return fmt.Errorf("qcow2: invalid compressed descriptor l1=%d l2=%d: %w", l1i, l2i, err)
				}
				if hostOff >= fileSize {
					return fmt.Errorf("qcow2: compressed data off past EOF: l1=%d l2=%d off=%d", l1i, l2i, hostOff)
				}
				if hostOff+compBytes > fileSize {
					return fmt.Errorf("qcow2: compressed data exceeds EOF: l1=%d l2=%d off=%d size=%d", l1i, l2i, hostOff, compBytes)
				}
				continue
			}

			if (l2e&qcowL2Zero) != 0 && (l2e&qcowL2OffsetMask) == 0 {
				continue
			}

			dataOff := l2e & qcowL2OffsetMask
			if dataOff == 0 {
				continue
			}
			if dataOff >= fileSize {
				return fmt.Errorf("qcow2: data off past EOF: l1=%d l2=%d off=%d", l1i, l2i, dataOff)
			}
			if dataOff+clusterSize > fileSize {
				return fmt.Errorf("qcow2: data cluster exceeds EOF: l1=%d l2=%d off=%d", l1i, l2i, dataOff)
			}
			if dataOff%clusterSize != 0 {
				return fmt.Errorf("qcow2: unaligned data cluster: l1=%d l2=%d off=%d", l1i, l2i, dataOff)
			}
		}
	}

	return nil
}
