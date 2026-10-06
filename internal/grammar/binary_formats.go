package grammar

import (
	"bytes"
	_ "embed"
	"encoding/hex"
	"fmt"
	"path"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed catalog/binary_formats.yml
var embeddedBinaryFormats []byte

type binaryFormatFile struct {
	Version int            `yaml:"version"`
	Formats []binaryFormat `yaml:"formats"`
}

// binaryFormat is one catalog entry: the extensions it answers to and the
// magic bytes its content must start with.
type binaryFormat struct {
	Name       string            `yaml:"name"`
	Extensions []string          `yaml:"extensions"`
	Signatures []formatSignature `yaml:"signatures"`
}

// formatSignature is a run of bytes (hex) the content holds at Offset.
type formatSignature struct {
	Offset int    `yaml:"offset"`
	Hex    string `yaml:"hex"`
	bytes  []byte
}

var (
	binaryFormatsOnce  sync.Once
	binaryFormatsByExt map[string][]formatSignature
	binaryFormatsErr   error
)

// loadBinaryFormats parses the embedded catalog strictly, once. A format
// without a signature, or a signature that is not hex, is a load error.
func loadBinaryFormats() (map[string][]formatSignature, error) {
	binaryFormatsOnce.Do(func() {
		var f binaryFormatFile
		dec := yaml.NewDecoder(bytes.NewReader(embeddedBinaryFormats))
		dec.KnownFields(true)
		if err := dec.Decode(&f); err != nil {
			binaryFormatsErr = fmt.Errorf("grammar binary formats: %w", err)
			return
		}
		byExt := map[string][]formatSignature{}
		for _, format := range f.Formats {
			if len(format.Signatures) == 0 {
				binaryFormatsErr = fmt.Errorf("grammar binary formats: %s has no signature", format.Name)
				return
			}
			for i, sig := range format.Signatures {
				raw, err := hex.DecodeString(strings.TrimSpace(sig.Hex))
				if err != nil || len(raw) == 0 || sig.Offset < 0 {
					binaryFormatsErr = fmt.Errorf("grammar binary formats: %s signature %q: invalid", format.Name, sig.Hex)
					return
				}
				format.Signatures[i].bytes = raw
			}
			for _, ext := range format.Extensions {
				if ext = strings.ToLower(strings.TrimSpace(ext)); ext != "" {
					byExt[ext] = append(byExt[ext], format.Signatures...)
				}
			}
		}
		binaryFormatsByExt = byExt
	})
	return binaryFormatsByExt, binaryFormatsErr
}

// DeclaredBinaryFormat reports whether the file name with content is a binary
// format the catalog (catalog/binary_formats.yml) lists as holding no
// reviewable source: its extension (any case) is a listed format's, its
// content starts with one of that format's signatures, and the content is
// binary. A renamed script (payload.png without the PNG signature), an
// unlisted extension, or a catalog that cannot be read answers false, so the
// file stays unreviewed (fail-closed), never ignored.
func DeclaredBinaryFormat(name string, content []byte) bool {
	byExt, err := loadBinaryFormats()
	if err != nil {
		return false
	}
	ext := strings.TrimPrefix(strings.ToLower(path.Ext(name)), ".")
	if ext == "" || !LooksBinary(content) {
		return false
	}
	for _, sig := range byExt[ext] {
		if signatureMatches(content, sig) {
			return true
		}
	}
	return false
}

// signatureMatches reports whether content holds sig's bytes at its offset.
func signatureMatches(content []byte, sig formatSignature) bool {
	end := sig.Offset + len(sig.bytes)
	return end <= len(content) && bytes.Equal(content[sig.Offset:end], sig.bytes)
}
