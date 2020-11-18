package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
)

type LocalImageSource struct {
	fs             http.FileSystem
	maxSize        int64
	allowedFormats []string
	bufPool        chan []byte
}

func NewLocalImageSource(fs http.FileSystem, maxSize int64, allowedFormats []string, poolSize int) LocalImageSource {

	// These small buffers are used to read in the first few bytes of a received image and
	//  determine the file type using DetectContentType
	bufPool := make(chan []byte, poolSize)
	for i := 0; i < cap(bufPool); i++ {
		buffer := make([]byte, 512)
		bufPool <- buffer
	}

	return LocalImageSource{fs, maxSize, allowedFormats, bufPool}
}

func (is LocalImageSource) Read(name string) ([]byte, error) {
	// Open the file from the local filesystem for reading
	file, err := is.fs.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { err = file.Close() }() // Make sure we actually close the file

	// Read file information for later use
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}

	// Throw a 404 if someone tries to read a directory. We also don't care about any index.html file serving
	if info.IsDir() {
		fmt.Printf("Trying to read from directory\n")
		return nil, os.ErrNotExist
	}

	// Reject resizing any images that are too large
	if info.Size() > is.maxSize {
		// TODO: Maybe this should be a different error, since it /does/ exist, but we
		//  probably don't want to leak state
		fmt.Printf("Image was too large, not loading...")
		return nil, os.ErrNotExist
	}

	// TODO: Before we read in the file, maybe we should check if it's actually an image.
	//  Perhaps this could be accomplished with DetectContentType, reading in 512 bytes only.
	// <https://stackoverflow.com/questions/25959386/how-to-check-if-a-file-is-a-valid-image>
	// <https://socketloop.com/tutorials/golang-how-to-verify-uploaded-file-is-image-or-allowed-file-types>

	// Decoder wants []byte, so read the whole file into a buffer
	inputBuf, err := ioutil.ReadAll(file)
	if err != nil {
		fmt.Printf("Failed to read input file, %s\n", err)
		return nil, err // TODO: We might not want to leak state here, maybe just send a 404
	}

	return inputBuf, nil
}
