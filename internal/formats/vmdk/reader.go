package vmdk

import (
	"fmt"
	"os"
)

type Reader struct {
	spans   []extentSpan
	size    uint64
	closers []extent
}

func OpenRange(path string) (*Reader, error) {
	isSparse, err := hasSparseMagic(path)
	if err != nil {
		return nil, err
	}
	if isSparse {
		return openSingleExtent(path)
	}

	isDesc, err := looksLikeTextDescriptor(path)
	if err != nil {
		return nil, err
	}
	if !isDesc {
		return nil, fmt.Errorf("%w", ErrUnsupportedVMDK)
	}

	d, err := parseDescriptor(path)
	if err != nil {
		return nil, err
	}

	if len(d.extents) == 0 {
		return openSingleExtent(path)
	}

	var spans []extentSpan
	var closers []extent
	var cur uint64

	for _, ex := range d.extents {
		ep := resolveExtentPath(path, ex.fileName)

		var ext extent
		switch ex.typ {
		case extentFlat:
			ext, err = openFlatExtent(ep, ex.sectors, ex.flatOffset)
		case extentSparse, extentStream:
			ext, err = openExtentByHeader(ep)
		default:
			return nil, fmt.Errorf("%w: extent type %q", ErrUnsupportedVMDK, ex.typ)
		}
		if err != nil {
			for _, c := range closers {
				_ = c.Close()
			}
			return nil, err
		}

		sz := ex.sectors * sectorSize
		spans = append(spans, extentSpan{
			start: cur,
			end:   cur + sz,
			ext:   ext,
		})
		closers = append(closers, ext)
		cur += sz
	}

	return &Reader{spans: spans, size: cur, closers: closers}, nil
}

func openSingleExtent(path string) (*Reader, error) {
	ext, err := openExtentByHeader(path)
	if err != nil {
		return nil, err
	}
	sz := ext.Size()
	return &Reader{
		spans:   []extentSpan{{start: 0, end: sz, ext: ext}},
		size:    sz,
		closers: []extent{ext},
	}, nil
}

func openExtentByHeader(path string) (extent, error) {
	isSparse, err := hasSparseMagic(path)
	if err != nil {
		return nil, err
	}
	if !isSparse {
		return nil, fmt.Errorf("%w: cannot open non-sparse extent without descriptor", ErrUnsupportedVMDK)
	}

	ff, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	h, err := readSparseHeader(ff)
	_ = ff.Close()
	if err != nil {
		return nil, err
	}

	debugProbeVMDK(path, h)

	dbg("openExtentByHeader: layout=%d cap=%d grain=%d gtes=%d overHead=%d gd=%d rgd=%d",
		h.Layout(), h.CapacitySectors, h.GrainSizeSectors, h.NumGTEsPerGT,
		h.OverHeadSectors, h.GdOffset, h.RgdOffset)

	switch h.Layout() {

	case layoutHostedSparse:
		dbg("using hostedSparse backend")
		return openHostedSparseExtent(path)

	case layoutStreamOptimized:
		// Stream-optimized compressed sparse extents always use a marker stream
		// (Virtual Disk Format 1.1, p.14-17). The header typically has gdOffset=GD_AT_END
		// so GD/GT offsets in the header are not directly usable.
		dbg("using streamOptimized marker-stream backend")
		return openStreamOptimizedExtent(path)

	default:
		return nil, fmt.Errorf("%w: unsupported sparse layout", ErrUnsupportedVMDK)
	}
}

func (r *Reader) Close() error {
	var first error
	for _, c := range r.closers {
		if err := c.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (r *Reader) Size() uint64 { return r.size }

func (r *Reader) ReadAt(p []byte, off int64) (int, error) {
	return readAcrossSpans(r.spans, p, off, r.size)
}

func Open(path string) (*Reader, error) { return OpenRange(path) }
