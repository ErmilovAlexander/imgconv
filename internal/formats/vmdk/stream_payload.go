package vmdk

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
)

type payloadDesc struct {
	offBytes  int64
	sizeBytes int64
	zero      bool

	// debug/trace
	lbaSectors uint64
	markerOff  int64
	dupCount   int
}

type streamGrainMap struct {
	grains map[uint64]payloadDesc
}

func buildStreamGrainMap(events []streamEvent, h *sparseHeader) (*streamGrainMap, error) {
	grainSectors := uint64(h.GrainSizeSectors)
	if grainSectors == 0 {
		return nil, fmt.Errorf("invalid GrainSizeSectors")
	}

	m := &streamGrainMap{grains: make(map[uint64]payloadDesc, 4096)}

	// count duplicates per grain to debug "last-wins" behavior
	dup := make(map[uint64]int, 4096)

	for _, ev := range events {
		if ev.Type != streamEventDATA {
			continue
		}
		if ev.LBA%grainSectors != 0 {
			if debugOn() {
				dbg("streamOptimized: WARN: data marker LBA not grain-aligned: lba=%d grainSectors=%d (markerOff=%d)", ev.LBA, grainSectors, ev.MarkerOff)
			}
			continue
		}
		grainIdx := ev.LBA / grainSectors
		dup[grainIdx]++

		// IMPORTANT: keep LAST occurrence for same grain (overwrite)
		m.grains[grainIdx] = payloadDesc{
			offBytes:   ev.PayloadOff,
			sizeBytes:  int64(ev.SizeBytes), // BYTES
			zero:       false,
			lbaSectors: ev.LBA,
			markerOff:  ev.MarkerOff,
			dupCount:   dup[grainIdx],
		}
	}

	// copy final dupCount into stored payloadDesc (so it reflects total count)
	for g, pd := range m.grains {
		pd.dupCount = dup[g]
		m.grains[g] = pd
	}

	return m, nil
}

func (m *streamGrainMap) get(grainIdx uint64) (payloadDesc, bool) {
	p, ok := m.grains[grainIdx]
	return p, ok
}

func readPayload(f *os.File, p payloadDesc, grainBytes uint64) ([]byte, error) {
	if p.zero || p.sizeBytes <= 0 {
		return make([]byte, grainBytes), nil
	}

	raw := make([]byte, p.sizeBytes)
	if _, err := f.ReadAt(raw, p.offBytes); err != nil {
		return nil, fmt.Errorf("read payload: %w", err)
	}

	// === CRITICAL FIX ===
	// В streamOptimized payload может занимать РОВНО размер grainBytes,
	// но при этом быть ZLIB/DEFLATE потоком, упакованным в 64K с паддингом.
	// Поэтому: если похоже на zlib/deflate — всегда пытаемся распаковать,
	// даже когда len(raw)==grainBytes.
	if looksLikeZlib(raw) {
		if out, ok, err := tryInflateZlib(raw, int(grainBytes)); err != nil {
			return nil, err
		} else if ok {
			return out, nil
		}
		// fallback: иногда встречается raw-deflate без zlib-обёртки,
		// но заголовок может совпасть — попробуем и его.
		if out, ok, err := tryInflateRaw(raw, int(grainBytes)); err != nil {
			return nil, err
		} else if ok {
			return out, nil
		}
		// если не распаковывается — считаем, что это реально несжатый блок.
	}

	// Full-grain stored uncompressed (common case)
	if uint64(len(raw)) == grainBytes {
		return raw, nil
	}

	// zlib (обычный случай)
	if out, ok, err := tryInflateZlib(raw, int(grainBytes)); err != nil {
		return nil, err
	} else if ok {
		return out, nil
	}

	// raw deflate
	if out, ok, err := tryInflateRaw(raw, int(grainBytes)); err != nil {
		return nil, err
	} else if ok {
		return out, nil
	}

	// Rare: "raw bytes but smaller than grain" -> treat as prefix and zero-tail
	if len(raw) > 0 && !looksLikeZlib(raw) && uint64(len(raw)) < grainBytes {
		buf := make([]byte, grainBytes)
		copy(buf, raw)
		return buf, nil
	}

	return nil, fmt.Errorf("%w: payload not zlib/deflate (len=%d wantGrain=%d)", ErrUnsupportedVMDK, len(raw), grainBytes)
}

func looksLikeZlib(b []byte) bool {
	if len(b) < 2 {
		return false
	}
	// CMF/FLG check: compression method deflate (8) and header checksum.
	cmf := b[0]
	flg := b[1]
	if (cmf & 0x0F) != 8 {
		return false
	}
	// zlib header checksum (CMF*256+FLG) % 31 == 0
	if (int(cmf)*256+int(flg))%31 != 0 {
		return false
	}
	return true
}

func tryInflateZlib(raw []byte, want int) ([]byte, bool, error) {
	r, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, false, nil
	}
	defer r.Close()

	out, err := io.ReadAll(r)
	if err != nil {
		return nil, false, fmt.Errorf("zlib inflate error: %w", err)
	}

	// Важно: иногда поток короче, остальное = нули.
	if len(out) != want {
		if len(out) < want {
			buf := make([]byte, want)
			copy(buf, out)
			return buf, true, nil
		}
		return nil, false, fmt.Errorf("zlib inflate produced %d bytes, want %d", len(out), want)
	}

	return out, true, nil
}

func tryInflateRaw(raw []byte, want int) ([]byte, bool, error) {
	r := flate.NewReader(bytes.NewReader(raw))
	defer r.Close()

	out, err := io.ReadAll(r)
	if err != nil {
		return nil, false, nil
	}
	if len(out) != want {
		if len(out) < want {
			buf := make([]byte, want)
			copy(buf, out)
			return buf, true, nil
		}
		return nil, false, nil
	}
	return out, true, nil
}

// helper for debug printing of CRC32 (used from stream.go)
func crc32Of(b []byte) uint32 { return crc32.ChecksumIEEE(b) }

// helper for debug: read first 4 bytes of compressed payload to see headers
func payloadPrefix4(f *os.File, off int64) (uint32, error) {
	var tmp [4]byte
	if _, err := f.ReadAt(tmp[:], off); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(tmp[:]), nil
}
