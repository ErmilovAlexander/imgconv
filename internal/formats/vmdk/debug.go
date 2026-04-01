package vmdk

import (
	"fmt"
	"os"
	"sync/atomic"
)

var debugEnabled atomic.Bool

func SetDebug(v bool) {
	debugEnabled.Store(v)
}

func debugOn() bool {
	return debugEnabled.Load()
}

func dbg(format string, args ...any) {
	if !debugOn() {
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, "DEBUG vmdk: "+format+"\n", args...)
}

// dbgh always prints when debug is enabled, and prefixes section header.
func dbgh(section string, format string, args ...any) {
	if !debugOn() {
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, "DEBUG vmdk [%s]: "+format+"\n", append([]any{section}, args...)...)
}
