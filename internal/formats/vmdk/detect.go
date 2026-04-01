package vmdk

import (
	"bytes"
	"os"
)

func looksLikeTextDescriptor(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	buf := make([]byte, 4096)
	n, _ := f.Read(buf)
	buf = buf[:n]

	// Если много нулей — это почти точно бинарь (extent), не descriptor.
	if bytes.Count(buf, []byte{0x00}) > 8 {
		return false, nil
	}

	// Типичные признаки descriptor
	if bytes.Contains(buf, []byte("createType=")) ||
		bytes.Contains(buf, []byte("version=")) ||
		bytes.Contains(buf, []byte("\nRW ")) ||
		bytes.HasPrefix(bytes.TrimSpace(buf), []byte("# Disk DescriptorFile")) {
		return true, nil
	}

	return false, nil
}
