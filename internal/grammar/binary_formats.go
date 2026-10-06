package grammar

import (
	"bytes"
	_ "embed"
	"fmt"
	"path"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed catalog/binary_formats.yml
var embeddedBinaryFormats []byte

type binaryFormatFile struct {
	Version    int      `yaml:"version"`
	Extensions []string `yaml:"extensions"`
}

var (
	binaryFormatsOnce sync.Once
	binaryFormats     map[string]bool
	binaryFormatsErr  error
)

// loadBinaryFormats parses the embedded catalog strictly, once.
func loadBinaryFormats() (map[string]bool, error) {
	binaryFormatsOnce.Do(func() {
		var f binaryFormatFile
		dec := yaml.NewDecoder(bytes.NewReader(embeddedBinaryFormats))
		dec.KnownFields(true)
		if err := dec.Decode(&f); err != nil {
			binaryFormatsErr = fmt.Errorf("grammar binary formats: %w", err)
			return
		}
		binaryFormats = map[string]bool{}
		for _, ext := range f.Extensions {
			if ext = strings.ToLower(strings.TrimSpace(ext)); ext != "" {
				binaryFormats[ext] = true
			}
		}
	})
	return binaryFormats, binaryFormatsErr
}

// KnownBinaryFormat reports whether path names a file of a binary format the
// catalog (catalog/binary_formats.yml) lists as holding no reviewable source.
// It decides from the name only; the caller also requires binary content
// (LooksBinary). A catalog that cannot be read answers false, so the file
// stays unreviewed (fail-closed), never ignored.
func KnownBinaryFormat(name string) bool {
	formats, err := loadBinaryFormats()
	if err != nil {
		return false
	}
	ext := strings.TrimPrefix(strings.ToLower(path.Ext(name)), ".")
	return ext != "" && formats[ext]
}
