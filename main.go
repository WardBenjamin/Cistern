package main

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
	fs http.FileSystem
}

//
func (fs thumbnailFileSystem) Open(name string) (http.File, error) {

	file, err := fs.fs.Open(name)
	if err != nil {
		return nil, err
	}

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

	// Create a buffer to store the output image, 20MB in this case
	// TODO: Replace this with a pool or some other way to allocate blocked out memory
	outputBuf := make([]byte, 50*1024*1024)

	err = processThumb(inputBuf, outputBuf)
	if err != nil {
		fmt.Printf("failed to profess thumbnail, %s\n", err)
		return nil, err // TODO: We might not want to leak state here, maybe just send a 404
	}

	fmt.Printf("finishing file open\n")
	return virtualfile.OpenVirtualFile(outputBuf, name), nil
}

func processThumb(inputBuf []byte, outputBuf []byte) (error) {

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

	// Get ready to resize image, using 8192x8192 maximum resize buffer size
	ops := lilliput.NewImageOps(8192)
	defer ops.Close()

	opts := &lilliput.ImageOptions{
		FileType:             outputType,
		Width:                270,
		Height:               270,
		ResizeMethod:         resizeMethod,
		NormalizeOrientation: true,
		EncodeOptions:        EncodeOptions[outputType],
	}

	// resize and transcode image
	outputBuf, err = ops.Transform(decoder, opts, outputBuf)
	if err != nil {
		fmt.Printf("error transforming image, %s\n", err)
		return err
	}

	fmt.Printf("image successfully transformed\n")
	return nil
}

func main() {
	var imageRoot string

	flag.StringVar(&imageRoot, "root", "", "absolute path to image storage")
	flag.Parse()

	if imageRoot == "" {
		fmt.Printf("No image root path provided, quitting.\n")
		flag.Usage()
		os.Exit(1)
	}

	// See Example(DotFileHiding): https://golang.org/pkg/net/http/#FileServer
	// See Disable directory listing with http.FileServer: https://groups.google.com/u/2/g/golang-nuts/c/bStLPdIVM6w
	// See Prevent Access to Files in Folder: https://stackoverflow.com/questions/40716869/prevent-access-to-files-in-folder-with-a-golang-server
	fs := thumbnailFileSystem{http.Dir(imageRoot)}

	// See Example(StripPrefix): https://golang.org/pkg/net/http/#FileServer
	http.Handle("/thumb/", http.StripPrefix("/thumb/", http.FileServer(fs)))
	log.Fatal(http.ListenAndServe(":8080", nil))

	//
	////// image has been resized, now write file out
	////if outputFilename == "" {
	////	outputFilename = "resized" + filepath.Ext(inputFilename)
	////}
	////
	////if _, err := os.Stat(outputFilename); !os.IsNotExist(err) {
	////	fmt.Printf("output filename %s exists, quitting\n", outputFilename)
	////	os.Exit(1)
	////}
	////
	////err = ioutil.WriteFile(outputFilename, outputImg, 0400)
	////if err != nil {
	////	fmt.Printf("error writing out resized image, %s\n", err)
	////	os.Exit(1)
	////}
	//
	//fmt.Printf("image written to %s\n", outputFilename)
}
