package qcow2

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

const (
	magicQcow2 = 0x514649fb

	version2 = 2
	version3 = 3

	// Incompatible feature bits
	incompatDirtyBit           = uint64(1) << 0
	incompatCorruptBit         = uint64(1) << 1
	incompatExternalDataBit    = uint64(1) << 2
	incompatCompressionTypeBit = uint64(1) << 3
	incompatExtendedL2Bit      = uint64(1) << 4

	// Compression types
	compressionDeflate = 0
	compressionZstd    = 1
)

type Header struct {
	Magic                 uint32
	Version               uint32
	BackingFileOffset     uint64
	BackingFileSize       uint32
	ClusterBits           uint32
	Size                  uint64
	CryptMethod           uint32
	L1Size                uint32
	L1TableOffset         uint64
	RefcountTableOffset   uint64
	RefcountTableClusters uint32
	NbSnapshots           uint32
	SnapshotsOffset       uint64
	IncompatibleFeatures  uint64
	CompatibleFeatures    uint64
	AutoclearFeatures     uint64
	RefcountOrder         uint32
	HeaderLength          uint32
	CompressionType       uint8

	// Compatibility aliases for older code paths.
	RefcountOffset   uint64
	RefcountClusters uint32
}

func (h *Header) normalizeCompat() {
	if h.RefcountTableOffset == 0 && h.RefcountOffset != 0 {
		h.RefcountTableOffset = h.RefcountOffset
	}
	if h.RefcountOffset == 0 && h.RefcountTableOffset != 0 {
		h.RefcountOffset = h.RefcountTableOffset
	}
	if h.RefcountTableClusters == 0 && h.RefcountClusters != 0 {
		h.RefcountTableClusters = h.RefcountClusters
	}
	if h.RefcountClusters == 0 && h.RefcountTableClusters != 0 {
		h.RefcountClusters = h.RefcountTableClusters
	}
}

func (h Header) ClusterSize() uint64 {
	return uint64(1) << h.ClusterBits
}

func ReadHeader(path string) (Header, error) {
	f, err := os.Open(path)
	if err != nil {
		return Header{}, err
	}
	defer f.Close()
	return readHeaderAt(f)
}

func readHeaderAt(r io.ReaderAt) (Header, error) {
	buf := make([]byte, 112)
	if _, err := r.ReadAt(buf, 0); err != nil && err != io.EOF {
		return Header{}, err
	}

	var h Header
	h.Magic = binary.BigEndian.Uint32(buf[0:4])
	if h.Magic != magicQcow2 {
		return Header{}, fmt.Errorf("qcow2: bad magic")
	}

	h.Version = binary.BigEndian.Uint32(buf[4:8])
	if h.Version != version2 && h.Version != version3 {
		return Header{}, fmt.Errorf("qcow2: unsupported version %d", h.Version)
	}

	h.BackingFileOffset = binary.BigEndian.Uint64(buf[8:16])
	h.BackingFileSize = binary.BigEndian.Uint32(buf[16:20])
	h.ClusterBits = binary.BigEndian.Uint32(buf[20:24])
	h.Size = binary.BigEndian.Uint64(buf[24:32])
	h.CryptMethod = binary.BigEndian.Uint32(buf[32:36])
	h.L1Size = binary.BigEndian.Uint32(buf[36:40])
	h.L1TableOffset = binary.BigEndian.Uint64(buf[40:48])
	h.RefcountTableOffset = binary.BigEndian.Uint64(buf[48:56])
	h.RefcountTableClusters = binary.BigEndian.Uint32(buf[56:60])
	h.NbSnapshots = binary.BigEndian.Uint32(buf[60:64])
	h.SnapshotsOffset = binary.BigEndian.Uint64(buf[64:72])

	if h.Version >= version3 {
		h.IncompatibleFeatures = binary.BigEndian.Uint64(buf[72:80])
		h.CompatibleFeatures = binary.BigEndian.Uint64(buf[80:88])
		h.AutoclearFeatures = binary.BigEndian.Uint64(buf[88:96])
		h.RefcountOrder = binary.BigEndian.Uint32(buf[96:100])
		h.HeaderLength = binary.BigEndian.Uint32(buf[100:104])

		if h.HeaderLength > 104 {
			h.CompressionType = buf[104]
		}
	} else {
		h.RefcountOrder = 4
		h.HeaderLength = 72
		h.CompressionType = compressionDeflate
	}

	if (h.IncompatibleFeatures & incompatCompressionTypeBit) == 0 {
		h.CompressionType = compressionDeflate
	}

	h.normalizeCompat()

	if h.ClusterBits < 9 {
		return Header{}, fmt.Errorf("qcow2: invalid cluster_bits=%d", h.ClusterBits)
	}
	if h.ClusterSize() == 0 {
		return Header{}, fmt.Errorf("qcow2: zero cluster size")
	}
	if h.Size == 0 {
		return Header{}, fmt.Errorf("qcow2: zero virtual size")
	}
	if h.L1Size == 0 {
		return Header{}, fmt.Errorf("qcow2: zero l1 size")
	}
	if h.L1TableOffset == 0 {
		return Header{}, fmt.Errorf("qcow2: zero l1 table offset")
	}

	return h, nil
}

func (h *Header) MarshalBinary() []byte {
	h.normalizeCompat()

	buf := make([]byte, 104)
	binary.BigEndian.PutUint32(buf[0:4], h.Magic)
	binary.BigEndian.PutUint32(buf[4:8], h.Version)
	binary.BigEndian.PutUint64(buf[8:16], h.BackingFileOffset)
	binary.BigEndian.PutUint32(buf[16:20], h.BackingFileSize)
	binary.BigEndian.PutUint32(buf[20:24], h.ClusterBits)
	binary.BigEndian.PutUint64(buf[24:32], h.Size)
	binary.BigEndian.PutUint32(buf[32:36], h.CryptMethod)
	binary.BigEndian.PutUint32(buf[36:40], h.L1Size)
	binary.BigEndian.PutUint64(buf[40:48], h.L1TableOffset)
	binary.BigEndian.PutUint64(buf[48:56], h.RefcountTableOffset)
	binary.BigEndian.PutUint32(buf[56:60], h.RefcountTableClusters)
	binary.BigEndian.PutUint32(buf[60:64], h.NbSnapshots)
	binary.BigEndian.PutUint64(buf[64:72], h.SnapshotsOffset)
	binary.BigEndian.PutUint64(buf[72:80], h.IncompatibleFeatures)
	binary.BigEndian.PutUint64(buf[80:88], h.CompatibleFeatures)
	binary.BigEndian.PutUint64(buf[88:96], h.AutoclearFeatures)
	binary.BigEndian.PutUint32(buf[96:100], h.RefcountOrder)
	binary.BigEndian.PutUint32(buf[100:104], h.HeaderLength)
	return buf
}

func (h *Header) WriteAt(w io.WriterAt, off int64) error {
	buf := h.MarshalBinary()
	_, err := w.WriteAt(buf, off)
	return err
}
