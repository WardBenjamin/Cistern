package virtualfile

// See https://stackoverflow.com/a/52697900/2253573 for a similar implementation

import (
	"bytes"
	"os"
)


type VirtualFile struct {
	*bytes.Reader
	vfi virtualFileInfo
}

func (vf VirtualFile) Close() error { return nil } // No-op, nothing to do here
func (vf VirtualFile) Readdir(count int) ([]os.FileInfo, error) {
	// We are only a single file, not a directory
	return nil, nil
}
func (vf VirtualFile) Stat() (os.FileInfo, error) {
	return vf.vfi, nil
}

func OpenVirtualFile(data []byte, filename string) VirtualFile {
	return VirtualFile{
		Reader: bytes.NewReader(data),
		vfi: virtualFileInfo{
			name: filename,
			data: data,
		},
	}
}
