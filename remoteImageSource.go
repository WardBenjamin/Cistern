package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

type RemoteImageSource struct {
	urlRoot        string
	maxSize        int64
	allowedFormats []string
	bufPool        chan []byte
}

func NewRemoteImageSource(urlRoot string, maxSize int64, allowedFormats []string, poolSize int) RemoteImageSource {

	// These small buffers are used to read in the first few bytes of a received image and
	//  determine the file type using DetectContentType
	bufPool := make(chan []byte, poolSize)
	for i := 0; i < cap(bufPool); i++ {
		buffer := make([]byte, 512)
		bufPool <- buffer
	}

	return RemoteImageSource{urlRoot, maxSize, allowedFormats, bufPool}
}

func (is RemoteImageSource) Read(name string) ([]byte, error) {

	// Construct URL with name and urlRoot
	// If the URL doesn't end with an image format (.png, .jpg, .webp, etc), stop
	// Validate with url.Parse
	// Perform the request
	// Examine the request header for image size, discard if it's too big (RIP bandwidth)
	// TODO: For debug, maintain a dictionary of failed URLs and amount of times failed?
	// Read in the first few bytes and examine content, make sure it is an image and matches the extension
	// Read the whole thing into a buffer and send it

	fmt.Printf("Reading image from %s\n", name)

	// TODO: This might not work at all with query parameters, but maybe that doesn't matter
	extension := filepath.Ext(name)
	if extension == "" {
		fmt.Printf("Unable to determine filename from extension\n")
		return nil, os.ErrNotExist
	}
	// <https://gist.github.com/akhenakh/8462840>
	mimeType := mime.TypeByExtension(extension)
	fmt.Printf("File should be of type %s\n", mimeType)

	// TODO: Bail out early if it's not in the list of allowed file types

	fmt.Printf("Trying %s\n", is.urlRoot+name)

	imageUrl, err := url.Parse(is.urlRoot + name)
	if err != nil {
		fmt.Printf("Failed to parse url\n")
		log.Fatal(err)
	}

	// TODO: Look at content-type and content-length here
	//  If content-length is unset, be WARY
	response, err := http.Get(imageUrl.String())
	if err != nil {
		fmt.Printf("Failed to get resource at %s\n", imageUrl.String())
		fmt.Println(err)
		return nil, err
	}
	defer response.Body.Close()  // As soon as we bail out, the response body will be closed and prevent further data transfer if applicable

	// TODO: Look at image size here before further parsing (StatusRequestEntityTooLarge)

	buf := <-is.bufPool
	defer func() { is.bufPool <- buf }()

	// Read in the first few bytes of the file to check if it's a valid image and we can detect the file type
	// TODO: Do we really need this after all? If a bad actor submits malicious files then they will have
	//  the correct mime type, right?
	// <https://stackoverflow.com/questions/25959386/how-to-check-if-a-file-is-a-valid-image>
	// <https://socketloop.com/tutorials/golang-how-to-verify-uploaded-file-is-image-or-allowed-file-types>
	// TODO: Maybe we should ignore small/zero-size files? We can either use the return value here or get that separately
	_, err = response.Body.Read(buf)
	if err != nil {
		fmt.Println("Failed to read beginning of response body")
		fmt.Println(err)
		return nil, err
	}

	filetype := http.DetectContentType(buf)
	fmt.Printf("Detected filetype: %s\n", filetype)

	// If the file types don't match what we predicted, bail out.
	// If it does match, we know we're good to go
	if filetype != mimeType {
		fmt.Printf("Warning! Response returned type %s but we expected %s based on the URL", filetype, mimeType)
		return nil, http.ErrNotSupported
		// TODO: Can I use http.Error here somehow? Only if we can get a ResponseWriter in this context
	}


	// Decoder wants []byte, so read the whole file into a buffer
	// Limit to the appropriate number of bytes so that we don't accept large files (this could also
	//  be accomplished by a proxy)
	inputBuf, err := ioutil.ReadAll(io.LimitReader(response.Body, is.maxSize))
	if err != nil {
		fmt.Printf("Failed to read body from response, %s\n", err)
		return nil, err // TODO: We might not want to leak state here, maybe just send a 404
	}


	// TODO: Not sure if this is a good idea
	return append(buf, inputBuf...), nil
}
