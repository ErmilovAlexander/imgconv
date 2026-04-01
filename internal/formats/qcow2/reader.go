package qcow2

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"imgconv/internal/formats/raw"
	"imgconv/internal/formats/vdi"
	"imgconv/internal/formats/vmdk"
)

type rangeReader interface {
	ReadAt([]byte, int64) (int, error)
	Size() uint64
	Close() error
}

type Reader struct {
	f *os.File
	h Header

	path        string
	backingFile string
	backing     rangeReader

	clusterSize uint64
	l1          []uint64
}

func Open(path string) (*Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	h, err := readHeaderAt(f)
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	if (h.IncompatibleFeatures & incompatExtendedL2Bit) != 0 {
		_ = f.Close()
		return nil, fmt.Errorf("qcow2: extended l2 entries are not supported yet")
	}
	if (h.IncompatibleFeatures & incompatExternalDataBit) != 0 {
		_ = f.Close()
		return nil, fmt.Errorf("qcow2: external data file is not supported yet")
	}

	clusterSize := uint64(1) << h.ClusterBits

	l1 := make([]uint64, h.L1Size)
	buf := make([]byte, len(l1)*8)
	if _, err := f.ReadAt(buf, int64(h.L1TableOffset)); err != nil && err != io.EOF {
		_ = f.Close()
		return nil, err
	}
	for i := range l1 {
		l1[i] = binary.BigEndian.Uint64(buf[i*8:])
	}

	backingName, err := readBackingFileName(f, h)
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	var backing rangeReader
	if backingName != "" {
		resolved := backingName
		if !filepath.IsAbs(backingName) {
			resolved = filepath.Join(filepath.Dir(path), backingName)
		}
		backing, err = openBackingAny(resolved)
		if err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("qcow2: open backing %q: %w", resolved, err)
		}
	}

	return &Reader{
		f:           f,
		h:           h,
		path:        path,
		backingFile: backingName,
		backing:     backing,
		clusterSize: clusterSize,
		l1:          l1,
	}, nil
}

func (r *Reader) Close() error {
	if r.backing != nil {
		_ = r.backing.Close()
	}
	return r.f.Close()
}

func (r *Reader) Size() uint64 {
	return r.h.Size
}

func (r *Reader) ClusterBits() uint32 {
	return r.h.ClusterBits
}

func (r *Reader) L1Size() uint32 {
	return r.h.L1Size
}

func (r *Reader) CompressionType() uint8 {
	return r.h.CompressionType
}

func (r *Reader) BackingFile() string {
	return r.backingFile
}

func (r *Reader) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, fmt.Errorf("qcow2: negative offset")
	}
	if uint64(off) >= r.h.Size {
		return 0, io.EOF
	}

	max := r.h.Size - uint64(off)
	if uint64(len(p)) > max {
		p = p[:max]
	}

	read := 0
	for read < len(p) {
		curOff := uint64(off) + uint64(read)
		clusterIdx := curOff / r.clusterSize
		inCluster := curOff % r.clusterSize
		want := uint64(len(p) - read)
		if want > r.clusterSize-inCluster {
			want = r.clusterSize - inCluster
		}

		ref, err := r.lookupCluster(clusterIdx)
		if err != nil {
			return read, err
		}

		switch {
		case ref.zero:
			for i := 0; i < int(want); i++ {
				p[read+i] = 0
			}

		case !ref.allocated:
			if r.backing != nil {
				n, err := r.backing.ReadAt(p[read:read+int(want)], int64(curOff))
				read += n
				if err != nil && err != io.EOF {
					return read, err
				}
				if n < int(want) {
					for i := n; i < int(want); i++ {
						p[read-int(want)+i] = 0
					}
				}
				continue
			}
			for i := 0; i < int(want); i++ {
				p[read+i] = 0
			}

		case ref.compressed:
			clusterBuf, err := r.readCompressedCluster(ref.compressedDesc)
			if err != nil {
				return read, err
			}
			copy(p[read:read+int(want)], clusterBuf[inCluster:inCluster+want])

		default:
			tmp := make([]byte, want)
			_, err := r.f.ReadAt(tmp, int64(ref.dataOff+inCluster))
			if err != nil && err != io.EOF {
				return read, err
			}
			copy(p[read:], tmp)
		}

		read += int(want)
	}

	return read, nil
}

type clusterRef struct {
	allocated      bool
	zero           bool
	compressed     bool
	dataOff        uint64
	compressedDesc uint64
}

func (r *Reader) lookupCluster(clusterIdx uint64) (clusterRef, error) {
	entries := entriesPerL2(r.clusterSize)
	l1Idx := clusterIdx / entries
	l2Idx := clusterIdx % entries

	if l1Idx >= uint64(len(r.l1)) {
		return clusterRef{}, io.EOF
	}

	l1e := r.l1[l1Idx]
	if l1e == 0 {
		return clusterRef{allocated: false}, nil
	}

	l2Off := l1e & qcowL1OffsetMask

	l2buf := make([]byte, r.clusterSize)
	_, err := r.f.ReadAt(l2buf, int64(l2Off))
	if err != nil && err != io.EOF {
		return clusterRef{}, err
	}

	l2e := binary.BigEndian.Uint64(l2buf[l2Idx*8 : l2Idx*8+8])
	if l2e == 0 {
		return clusterRef{allocated: false}, nil
	}

	if (l2e & qcowOFCompressed) != 0 {
		return clusterRef{
			allocated:      true,
			compressed:     true,
			compressedDesc: l2e,
		}, nil
	}

	if (l2e&qcowL2Zero) != 0 && (l2e&qcowL2OffsetMask) == 0 {
		return clusterRef{
			allocated: false,
			zero:      true,
		}, nil
	}

	dataOff := l2e & qcowL2OffsetMask
	if dataOff == 0 {
		return clusterRef{allocated: false}, nil
	}

	return clusterRef{
		allocated: true,
		dataOff:   dataOff,
	}, nil
}

func (r *Reader) readCompressedCluster(desc uint64) ([]byte, error) {
	hostOff, compBytes, err := decodeCompressedDescriptor(desc, r.h.ClusterBits)
	if err != nil {
		return nil, err
	}

	if r.h.CompressionType != compressionDeflate {
		switch r.h.CompressionType {
		case compressionZstd:
			return nil, fmt.Errorf("qcow2: zstd compressed clusters are not supported yet")
		default:
			return nil, fmt.Errorf("qcow2: unsupported compression type %d", r.h.CompressionType)
		}
	}

	comp := make([]byte, compBytes)
	if _, err := r.f.ReadAt(comp, int64(hostOff)); err != nil && err != io.EOF {
		return nil, err
	}

	fr := flate.NewReader(bytes.NewReader(comp))
	defer fr.Close()

	out := make([]byte, r.clusterSize)
	n, err := io.ReadFull(fr, out)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("qcow2: deflate decompress failed: %w", err)
	}
	if uint64(n) != r.clusterSize {
		return nil, fmt.Errorf("qcow2: decompressed cluster too short: got=%d want=%d", n, r.clusterSize)
	}

	return out, nil
}

func decodeCompressedDescriptor(desc uint64, clusterBits uint32) (hostOff uint64, compBytes uint64, err error) {
	x := uint32(70) - clusterBits
	if x == 0 || x >= 62 {
		return 0, 0, fmt.Errorf("qcow2: invalid compressed descriptor geometry for cluster_bits=%d", clusterBits)
	}

	hostMask := (uint64(1) << x) - 1
	hostOff = desc & hostMask

	additionalSectors := (desc >> x) & ((uint64(1) << (62 - x)) - 1)
	sectorsUsed := additionalSectors + 1
	compBytes = sectorsUsed*512 - (hostOff & 511)

	if compBytes == 0 {
		return 0, 0, fmt.Errorf("qcow2: invalid compressed descriptor: zero compressed size")
	}

	return hostOff, compBytes, nil
}

func readBackingFileName(r io.ReaderAt, h Header) (string, error) {
	if h.BackingFileOffset == 0 || h.BackingFileSize == 0 {
		return "", nil
	}
	buf := make([]byte, h.BackingFileSize)
	if _, err := r.ReadAt(buf, int64(h.BackingFileOffset)); err != nil && err != io.EOF {
		return "", err
	}
	return string(buf), nil
}

func openBackingAny(path string) (rangeReader, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".qcow2":
		return Open(path)
	case ".raw", ".img", ".bin":
		return raw.Open(path)
	case ".vdi":
		return vdi.Open(path)
	case ".vmdk":
		return vmdk.Open(path)
	}

	if r, err := Open(path); err == nil {
		return r, nil
	}
	if r, err := raw.Open(path); err == nil {
		return r, nil
	}
	if r, err := vdi.Open(path); err == nil {
		return r, nil
	}
	if r, err := vmdk.Open(path); err == nil {
		return r, nil
	}

	return nil, fmt.Errorf("unsupported backing format for %q", path)
}
