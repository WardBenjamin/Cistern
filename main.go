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
	_ "net/http/pprof"
	"os"
	"strings"
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

	var imageSourceFlag string
	var rootPath string
	var poolSize int
	var maxImageSize int64
	var debugFs string

	// Capture configuration parameters from CLI flags
	// TODO: Add flags for remote URL and switch between local and remote sources
	flag.StringVar(&imageSourceFlag, "imageSource", "", "one of: local, remote")
	flag.StringVar(&rootPath, "rootPath", "", "base URL for remote source, or absolute path to local directory")
	flag.IntVar(&poolSize, "poolSize", 5, "number of image buffers to allocate")
	flag.Int64Var(&maxImageSize, "maxImageSize", 20*1024*1024, "maximum image size to accept for resize, in bytes")
	flag.StringVar(&debugFs, "debugFs", "", "serve images at / from this directory (for debugging only)")
	flag.Parse()

	if imageSourceFlag == "" {
		fmt.Printf("No image source specified (options: local, remote), quitting.\n")
		flag.Usage()
		os.Exit(1)
	}

	if rootPath == "" {
		fmt.Printf("No image root path/URL provided, quitting.\n")
		flag.Usage()
		os.Exit(1)
	}

	var imageSource ImageSource

	if imageSourceFlag == "local" {
		imageSource = LocalImageSource{
			fs:      http.Dir(rootPath),
			maxSize: maxImageSize,
		}
		fmt.Printf("Using local image source at %s\n", rootPath)
	} else if imageSourceFlag == "remote" {
		rootPath = strings.TrimRight(rootPath, "/")
		// TODO: Make the allowed formats list configurable or at least more obvious
		imageSource = NewRemoteImageSource(rootPath, maxImageSize, []string{"image/png", "image/jpg", "image/jpeg", "image/webp"}, poolSize)
		fmt.Printf("Using remote image source at %s\n", rootPath)
	} else {
		fmt.Printf("Invalid image source specified (options: local, remote), quitting\n")
		os.Exit(1)
	}

	thumbnailProcessor := NewThumbnailProcessor(encodeOptions, poolSize)

	// See Example(DotFileHiding): https://golang.org/pkg/net/http/#FileServer
	// See Disable directory listing with http.FileServer: https://groups.google.com/u/2/g/golang-nuts/c/bStLPdIVM6w
	// See Prevent Access to Files in Folder: https://stackoverflow.com/questions/40716869/prevent-access-to-files-in-folder-with-a-golang-server
	thumbnailFs := thumbnailFileSystem{thumbnailProcessor, imageSource}

	// See Example(StripPrefix): https://golang.org/pkg/net/http/#FileServer
	// <https://stackoverflow.com/questions/27945310/why-do-i-need-to-use-http-stripprefix-to-access-my-static-files/27946132#27946132>
	http.Handle("/thumb/", http.StripPrefix("/thumb/", http.FileServer(thumbnailFs)))

	if debugFs != "" {
		http.Handle("/", http.FileServer(http.Dir(debugFs)))
		fmt.Printf("Serving images from %s at '/' (meant for debug only!)\n", debugFs)
	}

	log.Fatal(http.ListenAndServe(":8080", nil))
}
