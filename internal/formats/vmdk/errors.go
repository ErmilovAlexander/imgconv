package vmdk

import "errors"

var (
	ErrNotVMDK             = errors.New("not a vmdk")
	ErrUnsupportedVMDK     = errors.New("unsupported vmdk variant")
	ErrDescriptorMalformed = errors.New("vmdk descriptor malformed")
	ErrShortRead           = errors.New("short read")

	// Более точные причины (удобно логировать/обрабатывать отдельно)
	ErrStreamOptimized = errors.New("vmdk streamOptimized is not implemented yet")
	ErrSESparse        = errors.New("vmdk sesparse is not implemented yet")
)
