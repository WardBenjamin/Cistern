package main

// https://blog.discord.com/how-discord-resizes-150-million-images-every-day-with-go-and-c-c9e98731c65d
// https://news.ycombinator.com/item?id=15696855
// For later reference: https://github.com/imgproxy/imgproxy

// TODO: Consider rewriting this using fasthttp <https://github.com/valyala/fasthttp>
// TODO: Look into how the http.FileServer safety guarantees work; we need them here

import (
	"flag"
	"fmt"
	"github.com/discordapp/lilliput"
	"log"
	"net/http"
	"os"

	_ "net/http/pprof"
)

type ImageSource interface {
	Read(name string) ([]byte, error)
}

type thumbnailFileSystem struct {
	tp ThumbnailProcessor
	is ImageSource
}


func (fs thumbnailFileSystem) Open(name string) (http.File, error) {

	// Read the image from the specified (remote or local) image source
	imageBuffer, err := fs.is.Read(name)
	if err != nil {
		fmt.Printf("Failed to read image from specified source\n")
		return nil, err
	}

	// Process the image into a thumbnail
	thumbnailFile, err := fs.tp.process(imageBuffer, name)
	if err != nil {
		fmt.Printf("Failed to profess thumbnail, %s\n", err)
		return nil, err // TODO: We might not want to leak state here, maybe just send a 404
	}

	fmt.Printf("Finishing file open\n")
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
	var maxImageSize int64

	// Capture configuration parameters from CLI flags
	// TODO: Add flags for remote URL and switch between local and remote sources
	flag.StringVar(&imageRoot, "imageRoot", "", "absolute path to image storage")
	flag.IntVar(&poolSize, "poolSize", 5, "number of image buffers to allocate")
	flag.Int64Var(&maxImageSize, "maxImageSize", 20*1024*1024, "maximum image size to accept for resize, in bytes")
	flag.Parse()

	if imageRoot == "" {
		fmt.Printf("No image root path provided, quitting.\n")
		flag.Usage()
		os.Exit(1)
	}

	dirFs := http.Dir(imageRoot)
	thumbnailProcessor := NewThumbnailProcessor(encodeOptions, poolSize)
	imageSource := LocalImageSource{
		fs:      dirFs,
		maxSize: maxImageSize,
	}

	// See Example(DotFileHiding): https://golang.org/pkg/net/http/#FileServer
	// See Disable directory listing with http.FileServer: https://groups.google.com/u/2/g/golang-nuts/c/bStLPdIVM6w
	// See Prevent Access to Files in Folder: https://stackoverflow.com/questions/40716869/prevent-access-to-files-in-folder-with-a-golang-server
	thumbnailFs := thumbnailFileSystem{thumbnailProcessor, imageSource}

	// See Example(StripPrefix): https://golang.org/pkg/net/http/#FileServer
	http.Handle("/thumb/", http.StripPrefix("/thumb/", http.FileServer(thumbnailFs)))
	log.Fatal(http.ListenAndServe(":8080", nil))
}
