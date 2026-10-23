// Package resource defines the editable resource contract shared by handlers.
package resource

import "encoding/json"

type Assets map[string][]byte

type Manifest struct {
	Handler    string          `json:"handler"`
	SourceFile string          `json:"source_file"`
	Metadata   json.RawMessage `json:"metadata"`
}

type Unpacked struct {
	Manifest     Manifest
	Translatable Assets
	Meta         Assets
}

type Handler interface {
	Name() string
	Patterns() []string
	Unpack(name string, data []byte) (Unpacked, error)
	Repack(manifest Manifest, translatable, meta Assets) ([]byte, error)
}
