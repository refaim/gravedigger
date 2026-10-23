package resource

import (
	"encoding/json"
	"testing"
)

func TestManifestJSONContract(t *testing.T) {
	want := `{"handler":"ExampleHandler","source_file":"TITLE1.DD2","metadata":{"width":320}}`
	m := Manifest{Handler: "ExampleHandler", SourceFile: "TITLE1.DD2", Metadata: json.RawMessage(`{"width":320}`)}
	data, err := json.Marshal(m)
	if err != nil || string(data) != want {
		t.Fatalf("marshal = %s, %v; want %s", data, err, want)
	}
	var decoded Manifest
	if err := json.Unmarshal(data, &decoded); err != nil || decoded.Handler != m.Handler || decoded.SourceFile != m.SourceFile || string(decoded.Metadata) != string(m.Metadata) {
		t.Fatalf("decode = %#v, %v", decoded, err)
	}
}

func TestUnpackedAssetsRemainSeparate(t *testing.T) {
	u := Unpacked{Translatable: Assets{"title.png": {1}}, Meta: Assets{"title.tree": {2}}}
	if u.Translatable["title.png"][0] != 1 || u.Meta["title.tree"][0] != 2 {
		t.Fatal("editable and metadata assets were mixed")
	}
}
