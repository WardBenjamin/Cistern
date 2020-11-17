package main

// https://blog.discord.com/how-discord-resizes-150-million-images-every-day-with-go-and-c-c9e98731c65d
// https://news.ycombinator.com/item?id=15696855
// For later reference: https://github.com/imgproxy/imgproxy

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/discordapp/lilliput"

	"github.com/WardBenjamin/Cistern/virtualfile"
)

import _ "net/http/pprof"

var EncodeOptions = map[string]map[int]int{
	".jpeg": map[int]int{lilliput.JpegQuality: 85},
	".png":  map[int]int{lilliput.PngCompression: 7},
	".webp": map[int]int{lilliput.WebpQuality: 85},
}

type thumbnailFileSystem struct {
	fs      http.FileSystem
	opsPool chan *lilliput.ImageOps
	bufPool chan []byte
}

// TODO: Rewrite this using fasthttp https://github.com/valyala/fasthttp
// TODO: Look into how the http.FileServer safety guarantees work; we need them here
func (fs thumbnailFileSystem) Open(name string) (http.File, error) {

	file, err := fs.fs.Open(name)
	if err != nil {
		return nil, err
	}

	// TODO: Refactor this so that we can just reject resizing any images that are too large
	// TODO: When this fetches images from a remote URL instead of from filesystem, we can do this based on image headers
	// Throw a 404 if someone tries to read a directory. We also don't care about any index.html file serving
	if info, err := file.Stat(); err == nil && info.IsDir() {
		fmt.Printf("trying to read from directory\n")
		return nil, os.ErrNotExist
	}

	// Decoder wants []byte, so read the whole file into a buffer
	inputBuf, err := ioutil.ReadAll(file)
	if err != nil {
		fmt.Printf("failed to read input file, %s\n", err)
		return nil, err // TODO: We might not want to leak state here, maybe just send a 404
	}

	outputBuf := <-fs.bufPool
	defer func() { fs.bufPool <- outputBuf }()

	// Grab an imageOps object from the opsPool
	ops := <-fs.opsPool
	defer func() { fs.opsPool <- ops }()

	err = processThumb(inputBuf, outputBuf, ops)
	if err != nil {
		fmt.Printf("failed to profess thumbnail, %s\n", err)
		return nil, err // TODO: We might not want to leak state here, maybe just send a 404
	}

	fmt.Printf("finishing file open\n")
	return virtualfile.OpenVirtualFile(outputBuf, name), nil
}

func processThumb(inputBuf []byte, outputBuf []byte, ops *lilliput.ImageOps) (error) {

	decoder, err := lilliput.NewDecoder(inputBuf)
	// This error reflects very basic checks, mostly just for the magic bytes of the file
	// to match known image formats
	if err != nil {
		fmt.Printf("error decoding image, %s\n", err)
		return err
	}
	defer decoder.Close()

	header, err := decoder.Header()
	// This error is much more comprehensive and reflects format errors
	if err != nil {
		fmt.Printf("error reading image header, %s\n", err)
		return err
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

	opts := &lilliput.ImageOptions{
		FileType:             outputType,
		Width:                270,
		Height:               270,
		ResizeMethod:         resizeMethod,
		NormalizeOrientation: true,
		EncodeOptions:        EncodeOptions[outputType],
	}

	// TODO: Do I need to close opts?

	// Resize and transcode image
	outputBuf, err = ops.Transform(decoder, opts, outputBuf)
	if err != nil {
		fmt.Printf("error transforming image, %s\n", err)
		return err
	}

	fmt.Printf("image successfully transformed\n")
	return nil
}

func main() {
	// TODO: Log 404 errors https://stackoverflow.com/questions/34017342/log-404-on-http-fileserver
	
	var imageRoot string
	var poolSize int

	flag.StringVar(&imageRoot, "imageRoot", "", "absolute path to image storage")
	flag.IntVar(&poolSize, "poolSize", 5, "number of image buffers to allocate")
	flag.Parse()

	if imageRoot == "" {
		fmt.Printf("No image root path provided, quitting.\n")
		flag.Usage()
		os.Exit(1)
	}

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

	// See Example(DotFileHiding): https://golang.org/pkg/net/http/#FileServer
	// See Disable directory listing with http.FileServer: https://groups.google.com/u/2/g/golang-nuts/c/bStLPdIVM6w
	// See Prevent Access to Files in Folder: https://stackoverflow.com/questions/40716869/prevent-access-to-files-in-folder-with-a-golang-server
	fs := thumbnailFileSystem{http.Dir(imageRoot), opsPool, bufPool}

	// See Example(StripPrefix): https://golang.org/pkg/net/http/#FileServer
	http.Handle("/thumb/", http.StripPrefix("/thumb/", http.FileServer(fs)))
	log.Fatal(http.ListenAndServe(":8080", nil))
}
