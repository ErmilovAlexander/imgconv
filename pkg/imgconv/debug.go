package imgconv

import "imgconv/internal/formats/vmdk"

// SetVMDKDebug enables or disables verbose VMDK debug logging.
// This is primarily intended for the CLI and for troubleshooting integrations.
func SetVMDKDebug(enabled bool) {
	vmdk.SetDebug(enabled)
}
