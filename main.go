package main

// https://blog.discord.com/how-discord-resizes-150-million-images-every-day-with-go-and-c-c9e98731c65d
// https://news.ycombinator.com/item?id=15696855
// For later reference: https://github.com/imgproxy/imgproxy

import (
	"flag"
	"fmt"
	"github.com/discordapp/lilliput"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	_ "net/http/pprof"
)

type thumbnailFileSystem struct {
	fs http.FileSystem
	tp ThumbnailProcessor
	// TODO: Add image source variable to hold remote vs local image source configuration
}

// TODO: Consider rewriting this using fasthttp https://github.com/valyala/fasthttp
// TODO: Look into how the http.FileServer safety guarantees work; we need them here
func (fs thumbnailFileSystem) Open(name string) (http.File, error) {

	// TODO: Move file opening/checking/reading code to localImageSource
	file, err := fs.fs.Open(name)
	if err != nil {
		return nil, err
	}
	// Make sure we actually close the file
	defer func() { err = file.Close() } ()

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

	thumbnailFile, err := fs.tp.process(inputBuf, name)
	if err != nil {
		fmt.Printf("failed to profess thumbnail, %s\n", err)
		return nil, err // TODO: We might not want to leak state here, maybe just send a 404
	}

	fmt.Printf("finishing file open\n")
	return thumbnailFile, nil
}

func main() {
	// TODO: Log 404 errors https://stackoverflow.com/questions/34017342/log-404-on-http-fileserver

	var encodeOptions = map[string]map[int]int{
		".jpeg": map[int]int{lilliput.JpegQuality: 85},
		".png":  map[int]int{lilliput.PngCompression: 7},
		".webp": map[int]int{lilliput.WebpQuality: 85},
	}

	var imageRoot string
	var poolSize int

	// Capture configuration parameters from CLI flags
	// TODO: Add flags for remote URL and switch between local and remote sources
	flag.StringVar(&imageRoot, "imageRoot", "", "absolute path to image storage")
	flag.IntVar(&poolSize, "poolSize", 5, "number of image buffers to allocate")
	flag.Parse()

	if imageRoot == "" {
		fmt.Printf("No image root path provided, quitting.\n")
		flag.Usage()
		os.Exit(1)
	}

	thumbnailProcessor := NewThumbnailProcessor(encodeOptions, poolSize)

	// See Example(DotFileHiding): https://golang.org/pkg/net/http/#FileServer
	// See Disable directory listing with http.FileServer: https://groups.google.com/u/2/g/golang-nuts/c/bStLPdIVM6w
	// See Prevent Access to Files in Folder: https://stackoverflow.com/questions/40716869/prevent-access-to-files-in-folder-with-a-golang-server
	fs := thumbnailFileSystem{http.Dir(imageRoot), thumbnailProcessor}

	// See Example(StripPrefix): https://golang.org/pkg/net/http/#FileServer
	http.Handle("/thumb/", http.StripPrefix("/thumb/", http.FileServer(fs)))
	log.Fatal(http.ListenAndServe(":8080", nil))
}
