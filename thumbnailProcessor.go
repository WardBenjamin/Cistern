package main

import (
	"fmt"
	"github.com/WardBenjamin/Cistern/virtualfile"
	"github.com/discordapp/lilliput"
	"net/http"
	"strings"
	"time"
)

type ThumbnailProcessor struct {
	encodeOptions map[string]map[int]int
	opsPool       chan *lilliput.ImageOps
	bufPool       chan []byte
}

func NewThumbnailProcessor(encodeOptions map[string]map[int]int, poolSize int) ThumbnailProcessor {
	opsPool := make(chan *lilliput.ImageOps, poolSize)
	for i := 0; i < cap(opsPool); i++ {
		// Get ready to resize image, using 8192x8192 maximum resize buffer size
		ops := lilliput.NewImageOps(8192)
		opsPool <- ops
	}

	bufPool := make(chan []byte, poolSize)
	for i := 0; i < cap(bufPool); i++ {
		// Create a buffer to store the output image, 20MB in this case
		// TODO: This should be of configurable size
		buffer := make([]byte, 20*1024*1024)
		bufPool <- buffer
	}

	return ThumbnailProcessor{encodeOptions, opsPool, bufPool}
}

func (tp ThumbnailProcessor) process(inputBuf []byte, name string) (http.File, error) {
	outputBuf := <-tp.bufPool
	// TODO: There feels like there might be a race condition here where an image can be written
	// 	to a buffer while still being sent from the buffer by the virtualFile
	defer func() { tp.bufPool <- outputBuf }()

	// Grab an imageOps object from the opsPool
	ops := <-tp.opsPool
	defer func() { tp.opsPool <- ops }()

	decoder, err := lilliput.NewDecoder(inputBuf)
	// This error reflects very basic checks, mostly just for the magic bytes of the file
	// to match known image formats
	if err != nil {
		fmt.Printf("error decoding image, %s\n", err)
		return nil, err
	}
	defer decoder.Close()

	header, err := decoder.Header()
	// This error is much more comprehensive and reflects format errors
	if err != nil {
		fmt.Printf("error reading image header, %s\n", err)
		return nil, err
	}

	// Print some basic info about the image
	fmt.Printf("file type: %s\n", decoder.Description())
	fmt.Printf("original size: %dpx x %dpx\n", header.Width(), header.Height())

	if decoder.Duration() != 0 {
		fmt.Printf("duration: %.2f s\n", float64(decoder.Duration())/float64(time.Second))
	}

	// Grab the current type from the image description, no transcoding
	// TODO: Consider transcoding options
	outputType := "." + strings.ToLower(decoder.Description())
	resizeMethod := lilliput.ImageOpsFit

	// TODO: Move this to tp?
	opts := &lilliput.ImageOptions{
		FileType:             outputType,
		Width:                270,
		Height:               270,
		ResizeMethod:         resizeMethod,
		NormalizeOrientation: true,
		EncodeOptions:        tp.encodeOptions[outputType],
	}

	// TODO: Do I need to close opts?

	// Resize and transcode image
	outputBuf, err = ops.Transform(decoder, opts, outputBuf)
	if err != nil {
		fmt.Printf("error transforming image, %s\n", err)
		return nil, err
	}

	fmt.Printf("image successfully transformed\n")
	return virtualfile.OpenVirtualFile(outputBuf, name), nil
}
