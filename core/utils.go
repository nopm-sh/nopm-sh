package core

import (
	"net/http"
	"os"
	"regexp"
)

func isVersion(v string) bool {
	m := regexp.MustCompile(`^([0-9]+\.)*[0-9]*$`)
	return m.MatchString(v)
}

func GetFileContentType(out *os.File) (string, error) {

	// Only the first 512 bytes are used to sniff the content type.
	buffer := make([]byte, 512)

	_, err := out.Read(buffer)
	if err != nil {
		return "", err
	}

	// Use the net/http package's handy DectectContentType function. Always returns a valid
	// content-type by returning "application/octet-stream" if no others seemed to match.
	contentType := http.DetectContentType(buffer)

	return contentType, nil
}
