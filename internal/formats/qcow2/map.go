package qcow2

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type MapKind string

const (
	MapKindData       MapKind = "data"
	MapKindZero       MapKind = "zero"
	MapKindHole       MapKind = "hole"
	MapKindBacking    MapKind = "backing"
	MapKindCompressed MapKind = "compressed"
)

type MapExtent struct {
	Start  uint64  `json:"start"`
	Length uint64  `json:"length"`
	Kind   MapKind `json:"kind"`
}

func MapFile(path string) ([]MapExtent, error) {
	r, err := Open(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	clusterSize := uint64(1) << r.ClusterBits()
	size := r.Size()
	if clusterSize == 0 {
		return nil, fmt.Errorf("qcow2: invalid cluster size")
	}

	var out []MapExtent
	for off := uint64(0); off < size; off += clusterSize {
		want := clusterSize
		if off+want > size {
			want = size - off
		}

		clusterIdx := off / clusterSize
		ref, err := r.lookupCluster(clusterIdx)
		if err != nil && err != io.EOF {
			return nil, err
		}

		kind := classifyMapKind(r, ref)
		if len(out) > 0 && out[len(out)-1].Kind == kind && out[len(out)-1].Start+out[len(out)-1].Length == off {
			out[len(out)-1].Length += want
			continue
		}

		out = append(out, MapExtent{
			Start:  off,
			Length: want,
			Kind:   kind,
		})
	}

	return out, nil
}

func classifyMapKind(r *Reader, ref clusterRef) MapKind {
	switch {
	case ref.zero:
		return MapKindZero
	case ref.compressed:
		return MapKindCompressed
	case ref.allocated:
		return MapKindData
	default:
		if r.backing != nil {
			return MapKindBacking
		}
		return MapKindHole
	}
}

func WriteMapJSON(w io.Writer, extents []MapExtent) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(extents)
}

func WriteMapText(w io.Writer, extents []MapExtent) error {
	for _, e := range extents {
		if _, err := fmt.Fprintf(w, "%-10s start=%d length=%d\n", e.Kind, e.Start, e.Length); err != nil {
			return err
		}
	}
	return nil
}

func CountKinds(extents []MapExtent) map[MapKind]uint64 {
	out := map[MapKind]uint64{}
	for _, e := range extents {
		out[e.Kind] += e.Length
	}
	return out
}

func SaveMapJSON(path string, extents []MapExtent) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return WriteMapJSON(f, extents)
}
