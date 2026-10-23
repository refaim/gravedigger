package graphics

import (
	"encoding/json"
	"strings"

	"github.com/refaim/gravedigger/internal/resource"
)

func unpacked(handler, source string, metadata any, translatable, meta resource.Assets) resource.Unpacked {
	// Internal metadata contains only JSON-compatible structs and primitive fields.
	data, _ := json.Marshal(metadata)
	return resource.Unpacked{Manifest: resource.Manifest{Handler: handler, SourceFile: source, Metadata: data}, Translatable: translatable, Meta: meta}
}

func stem(name string) string { return strings.TrimSuffix(strings.ToLower(name), ".dd2") }
