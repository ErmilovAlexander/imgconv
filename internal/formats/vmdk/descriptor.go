package vmdk

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type extentType string

const (
	extentSparse extentType = "SPARSE"
	extentFlat   extentType = "FLAT"
	extentStream extentType = "STREAMOPTIMIZED"
	extentSE     extentType = "SESparse"
)

type extentInfo struct {
	typ        extentType
	sectors    uint64
	fileName   string // relative to descriptor dir
	flatOffset uint64 // sectors
}

type descriptor struct {
	createType string
	extents    []extentInfo
}

func parseDescriptor(path string) (*descriptor, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	d := &descriptor{}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "createType") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				val := strings.TrimSpace(parts[1])
				val = strings.Trim(val, `"`)
				d.createType = val
			}
			continue
		}

		if strings.HasPrefix(line, "RW ") {
			ext, err := parseExtentLine(line)
			if err != nil {
				return nil, err
			}
			d.extents = append(d.extents, *ext)
		}
	}

	// inline monolithicSparse: extents могут отсутствовать
	if len(d.extents) == 0 {
		if d.createType == "monolithicSparse" || d.createType == "streamOptimized" {
			return d, nil
		}
		return nil, ErrDescriptorMalformed
	}

	return d, nil
}

func parseExtentLine(line string) (*extentInfo, error) {
	fields := splitPreservingQuotes(line)
	if len(fields) < 4 {
		return nil, fmt.Errorf("%w: bad extent line: %q", ErrDescriptorMalformed, line)
	}
	if fields[0] != "RW" {
		return nil, fmt.Errorf("%w: extent line must start with RW: %q", ErrDescriptorMalformed, line)
	}

	sectors, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: bad sectors: %v", ErrDescriptorMalformed, err)
	}

	typ := extentType(fields[2])
	file := strings.Trim(fields[3], `"`)

	ext := &extentInfo{
		typ:      typ,
		sectors:  sectors,
		fileName: file,
	}

	switch typ {
	case extentSparse, extentStream:
		return ext, nil
	case extentFlat:
		if len(fields) < 5 {
			return nil, fmt.Errorf("%w: FLAT extent requires offset: %q", ErrDescriptorMalformed, line)
		}
		off, err := strconv.ParseUint(fields[4], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: bad flat offset: %v", ErrDescriptorMalformed, err)
		}
		ext.flatOffset = off
		return ext, nil
	default:
		return nil, fmt.Errorf("%w: extent type %q not supported", ErrUnsupportedVMDK, typ)
	}
}

func resolveExtentPath(descriptorPath, extentFile string) string {
	dir := filepath.Dir(descriptorPath)
	return filepath.Join(dir, extentFile)
}

func splitPreservingQuotes(s string) []string {
	var out []string
	var cur strings.Builder
	inQuotes := false

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			inQuotes = !inQuotes
			cur.WriteByte(c)
		case ' ', '\t':
			if inQuotes {
				cur.WriteByte(c)
				continue
			}
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}
