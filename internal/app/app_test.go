package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/refaim/gravedigger/internal/resource"
)

type exampleHandler struct{ fail string }

func (exampleHandler) Name() string       { return "ExampleHandler" }
func (exampleHandler) Patterns() []string { return []string{"TITLE*.DD2"} }
func (h exampleHandler) Unpack(name string, data []byte) (resource.Unpacked, error) {
	if h.fail == "unpack" {
		return resource.Unpacked{}, errors.New("decode failed")
	}
	asset := "title.png"
	if h.fail == "path" {
		asset = "../escaped.png"
	}
	u := resource.Unpacked{
		Manifest:     resource.Manifest{Handler: h.Name(), SourceFile: name, Metadata: json.RawMessage(`{}`)},
		Translatable: resource.Assets{asset: data},
		Meta:         resource.Assets{"title.tree": []byte("tree")},
	}
	if h.fail == "json" {
		u.Manifest.Metadata = json.RawMessage(`{`)
	}
	if h.fail == "meta-path" {
		u.Meta["../escape"] = nil
	}
	if h.fail == "nil-meta" {
		u.Meta = nil
	}
	return u, nil
}

func (h exampleHandler) Repack(m resource.Manifest, translatable, meta resource.Assets) ([]byte, error) {
	if h.fail == "repack" {
		return nil, errors.New("encode failed")
	}
	if string(meta["title.tree"]) != "tree" {
		return nil, errors.New("missing tree")
	}
	return translatable["title.png"], nil
}

func fixture(t *testing.T) (App, afero.Fs, *bytes.Buffer) {
	t.Helper()
	fs := afero.NewMemMapFs()
	if err := fs.MkdirAll("game", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, "game/TITLE1.DD2", []byte("pixels"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, "game/LEVEL01.DD2", []byte("level"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := new(bytes.Buffer)
	return App{FS: fs, Out: out, Handlers: []resource.Handler{exampleHandler{}}}, fs, out
}

func TestUnpackRepack(t *testing.T) {
	a, fs, _ := fixture(t)
	if err := a.Run([]string{"unpack", "game", "editable"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"translatable/title.png", "meta/TITLE1/manifest.json", "meta/TITLE1/title.tree", "meta/LEVEL01.DD2"} {
		if _, err := fs.Stat(filepath.Join("editable", name)); err != nil {
			t.Fatal(name, err)
		}
	}
	if err := afero.WriteFile(fs, "editable/translatable/title.png", []byte("edited pixels"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.Run([]string{"repack", "editable", "result"}); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"TITLE1.DD2": "edited pixels", "LEVEL01.DD2": "level"} {
		got, err := afero.ReadFile(fs, filepath.Join("result", name))
		if err != nil || string(got) != want {
			t.Fatalf("%s = %s, %v", name, got, err)
		}
	}
}

func TestUsage(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"--version"}, {"-V"}, {"unpack", "--help"}, {"repack", "-h"}} {
		a, _, out := fixture(t)
		if err := a.Run(args); err != nil || !strings.Contains(out.String(), "gravedigger") {
			t.Fatalf("%v: %s, %v", args, out.String(), err)
		}
	}
	for _, args := range [][]string{nil, {"unknown"}, {"unpack"}, {"wrong", "game", "out"}, {"unpack", "game", "game"}, {"unpack", "game", "game/sub"}} {
		a, _, _ := fixture(t)
		if err := a.Run(args); err == nil {
			t.Fatalf("%v accepted", args)
		}
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestOutputFailuresAreReported(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"--help"}, {"unpack", "game", "editable"}} {
		t.Run(strings.Join(args, "-"), func(t *testing.T) {
			a, _, _ := fixture(t)
			want := errors.New("output disconnected")
			a.Out = failingWriter{want}
			if err := a.Run(args); !errors.Is(err, want) {
				t.Fatalf("output failure = %v, want %v", err, want)
			}
		})
	}
}

func TestUsageErrorHasNoTrailingNewline(t *testing.T) {
	a, _, _ := fixture(t)
	err := a.Run(nil)
	if err == nil || strings.HasSuffix(err.Error(), "\n") {
		t.Fatalf("invalid usage error: %q", err)
	}
}

func TestRejectedInputDoesNotWriteOutput(t *testing.T) {
	for _, failure := range []string{"unpack", "path", "meta-path", "json"} {
		a, fs, _ := fixture(t)
		a.Handlers = []resource.Handler{exampleHandler{fail: failure}}
		if err := a.Run([]string{"unpack", "game", "out"}); err == nil {
			t.Fatal("accepted", failure)
		}
		if ok, _ := afero.Exists(fs, "out"); ok {
			t.Fatal("wrote partial output")
		}
	}
}

func TestEmptyOutputAndIgnoredFiles(t *testing.T) {
	a, fs, _ := fixture(t)
	_ = fs.Mkdir("out", 0o755)
	_ = fs.Mkdir("game/sub.DD2", 0o755)
	_ = afero.WriteFile(fs, "game/README.txt", []byte("ignored"), 0o644)
	if err := a.Run([]string{"unpack", "game", "out"}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := afero.Exists(fs, "out/meta/README.txt"); ok {
		t.Fatal("copied non-game file")
	}
}

func TestHandlerWithNoMetadataAssets(t *testing.T) {
	a, fs, _ := fixture(t)
	a.Handlers = []resource.Handler{exampleHandler{fail: "nil-meta"}}
	if err := a.Run([]string{"unpack", "game", "out"}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := afero.Exists(fs, "out/meta/TITLE1/manifest.json"); !ok {
		t.Fatal("manifest missing")
	}
}

func TestAssetConflicts(t *testing.T) {
	assets := resource.Assets{"image": []byte("original")}
	if err := addAssets(assets, "", resource.Assets{"image": []byte("original")}); err != nil {
		t.Fatal(err)
	}
	if err := addAssets(assets, "", resource.Assets{"image": []byte("different")}); err == nil {
		t.Fatal("conflict accepted")
	}
	a, fs, _ := fixture(t)
	if err := a.Run([]string{"unpack", "game", "editable"}); err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{"handler":"ExampleHandler","source_file":"LEVEL01.DD2","metadata":{}}`)
	_ = afero.WriteFile(fs, "editable/meta/TITLE1/manifest.json", manifest, 0o644)
	if err := a.Run([]string{"repack", "editable", "out"}); err == nil {
		t.Fatal("conflicting output accepted")
	}
	if a.handler("MissingHandler", true) != nil {
		t.Fatal("unknown handler resolved")
	}
}

func TestRepackReadFailures(t *testing.T) {
	for _, name := range []string{"editable/meta", "editable/meta/TITLE1/manifest.json", "editable/translatable", "editable/translatable/title.png"} {
		a, fs, _ := fixture(t)
		if err := a.Run([]string{"unpack", "game", "editable"}); err != nil {
			t.Fatal(err)
		}
		a.FS = faultFS{Fs: fs, op: "open", name: name}
		if err := a.Run([]string{"repack", "editable", "out"}); err == nil {
			t.Fatal("accepted read failure", name)
		}
	}
}

func TestRepackRejectsUnsafeOriginal(t *testing.T) {
	a, fs, _ := fixture(t)
	_ = fs.MkdirAll("editable/meta", 0o755)
	_ = afero.WriteFile(fs, "editable/meta/bad:name", nil, 0o644)
	if err := a.Run([]string{"repack", "editable", "out"}); err == nil {
		t.Fatal("unsafe original accepted")
	}
}

func TestInputAndOutputValidation(t *testing.T) {
	for _, scenario := range []string{"missing", "file", "empty", "non-game", "nonempty-output", "output-file"} {
		t.Run(scenario, func(t *testing.T) {
			a, fs, _ := fixture(t)
			input := "game"
			switch scenario {
			case "missing":
				input = "missing"
			case "file":
				input = "game/TITLE1.DD2"
			case "empty":
				_ = fs.Mkdir("empty", 0o755)
				input = "empty"
			case "non-game":
				_ = fs.Mkdir("empty", 0o755)
				_ = afero.WriteFile(fs, "empty/readme.txt", nil, 0o644)
				input = "empty"
			case "nonempty-output":
				_ = fs.Mkdir("out", 0o755)
				_ = afero.WriteFile(fs, "out/translation", []byte("keep"), 0o644)
			case "output-file":
				_ = afero.WriteFile(fs, "out", nil, 0o644)
			}
			if err := a.Run([]string{"unpack", input, "out"}); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}

func TestRepackErrors(t *testing.T) {
	for _, scenario := range []string{"no-meta", "empty-meta", "bad-json", "bad-source", "unknown-handler", "repack"} {
		t.Run(scenario, func(t *testing.T) {
			a, fs, _ := fixture(t)
			if err := a.Run([]string{"unpack", "game", "editable"}); err != nil {
				t.Fatal(err)
			}
			manifest := "editable/meta/TITLE1/manifest.json"
			switch scenario {
			case "no-meta":
				_ = fs.RemoveAll("editable/meta")
			case "empty-meta":
				_ = fs.RemoveAll("editable/meta")
				_ = fs.Mkdir("editable/meta", 0o755)
			case "bad-json":
				_ = afero.WriteFile(fs, manifest, []byte("{"), 0o644)
			case "bad-source":
				_ = afero.WriteFile(fs, manifest, []byte(`{"handler":"ExampleHandler","source_file":"../escape.EXE"}`), 0o644)
			case "unknown-handler":
				a.Handlers = nil
			case "repack":
				a.Handlers = []resource.Handler{exampleHandler{fail: "repack"}}
			}
			if err := a.Run([]string{"repack", "editable", "out"}); err == nil {
				t.Fatal("accepted")
			}
			if ok, _ := afero.Exists(fs, "out"); ok {
				t.Fatal("wrote partial output")
			}
		})
	}
}

func TestOnlyOriginals(t *testing.T) {
	a, fs, _ := fixture(t)
	a.Handlers = nil
	if err := a.Run([]string{"unpack", "game", "editable"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run([]string{"repack", "editable", "out"}); err != nil {
		t.Fatal(err)
	}
	got, _ := afero.ReadFile(fs, "out/TITLE1.DD2")
	if string(got) != "pixels" {
		t.Fatal(string(got))
	}
}

// Faults model OS failures without depending on host permissions or platform.
type faultFS struct {
	afero.Fs
	op, name string
}

func (f faultFS) Open(name string) (afero.File, error) {
	if f.op == "open" && filepath.Clean(name) == filepath.Clean(f.name) {
		return nil, os.ErrPermission
	}
	return f.Fs.Open(name)
}

func (f faultFS) Stat(name string) (os.FileInfo, error) {
	if f.op == "stat" && filepath.Clean(name) == filepath.Clean(f.name) {
		return nil, os.ErrPermission
	}
	return f.Fs.Stat(name)
}

func (f faultFS) MkdirAll(name string, mode os.FileMode) error {
	if f.op == "mkdir" {
		return os.ErrPermission
	}
	return f.Fs.MkdirAll(name, mode)
}

func (f faultFS) OpenFile(name string, flag int, mode os.FileMode) (afero.File, error) {
	if f.op == "write" && flag&os.O_WRONLY != 0 {
		return nil, os.ErrPermission
	}
	return f.Fs.OpenFile(name, flag, mode)
}

func TestFilesystemFailures(t *testing.T) {
	for _, tc := range []struct{ op, path string }{{"stat", "game"}, {"stat", "out"}, {"open", "game"}, {"open", "game/TITLE1.DD2"}, {"mkdir", ""}, {"write", ""}} {
		a, fs, _ := fixture(t)
		a.FS = faultFS{Fs: fs, op: tc.op, name: tc.path}
		if err := a.Run([]string{"unpack", "game", "out"}); err == nil {
			t.Fatalf("accepted %v", tc)
		}
	}
}

func TestNilOutput(t *testing.T) {
	a, _, _ := fixture(t)
	a.Out = io.Discard
	if err := a.Run([]string{"--version"}); err != nil {
		t.Fatal(err)
	}
}
