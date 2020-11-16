package virtualfile

import (
	"os"
	"time"
)

type virtualFileInfo struct {
	name string
	data []byte
}

func (vfi virtualFileInfo) Name() string       { return vfi.name }
func (vfi virtualFileInfo) Size() int64        { return int64(len(vfi.data)) }
func (vfi virtualFileInfo) Mode() os.FileMode  { return 0444 }        // Read for all
func (vfi virtualFileInfo) ModTime() time.Time { return time.Time{} } // Return anything
func (vfi virtualFileInfo) IsDir() bool        { return false }
func (vfi virtualFileInfo) Sys() interface{}   { return nil }
