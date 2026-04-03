package imgconv

import "errors"

var (
	ErrUnsupportedFormat = errors.New("imgconv: unsupported format")
	ErrInvalidArgument   = errors.New("imgconv: invalid argument")
	ErrOperationFailed   = errors.New("imgconv: operation failed")
)
