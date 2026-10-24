// Package app implements the file-based command line workflow.
package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/afero"

	"github.com/refaim/gravedigger/internal/resource"
)

const (
	Version = "3.1.0"
	usage   = "gravedigger — Dangerous Dave in the Haunted Mansion translation toolkit\nUsage: gravedigger unpack <game_dir> <output_dir>\n       gravedigger repack <editable_dir> <output_dir>\n"
)

type App struct {
	FS       afero.Fs
	Out      io.Writer
	Handlers []resource.Handler
}

func (a App) Run(args []string) error {
	if len(args) == 1 && (args[0] == "--version" || args[0] == "-V") {
		_, err := fmt.Fprintln(a.Out, "gravedigger", Version)
		return err
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") || len(args) == 2 && (args[0] == "unpack" || args[0] == "repack") && (args[1] == "--help" || args[1] == "-h") {
		_, err := fmt.Fprint(a.Out, usage)
		return err
	}
	if len(args) != 3 || (args[0] != "unpack" && args[0] != "repack") {
		return errors.New(strings.TrimSpace(usage))
	}
	input, output := filepath.Clean(args[1]), filepath.Clean(args[2])
	// Compare absolute paths to catch aliases such as game/../game. Reject both
	// directions so extraction can never overwrite its inputs or translations.
	inAbs, _ := filepath.Abs(input)
	outAbs, _ := filepath.Abs(output)
	inAbs, outAbs = strings.ToLower(inAbs), strings.ToLower(outAbs)
	if inAbs == outAbs || strings.HasPrefix(outAbs, inAbs+string(filepath.Separator)) || strings.HasPrefix(inAbs, outAbs+string(filepath.Separator)) {
		return errors.New("input and output directories must not overlap")
	}
	info, err := a.FS.Stat(input)
	if err != nil {
		return fmt.Errorf("input directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("input is not a directory: %s", input)
	}
	if err := a.validateOutput(output); err != nil {
		return err
	}
	var files resource.Assets
	if args[0] == "unpack" {
		files, err = a.unpack(input)
	} else {
		files, err = a.repack(input)
	}
	if err != nil {
		return err
	}
	for _, name := range sortedNames(files) {
		target := filepath.Join(output, filepath.FromSlash(name))
		if err := a.FS.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
		if err := afero.WriteFile(a.FS, target, files[name], 0o644); err != nil {
			return fmt.Errorf("write %s: %w", target, err)
		}
	}
	_, err = fmt.Fprintf(a.Out, "%s: wrote %d files to %s\n", args[0], len(files), output)
	return err
}

func (a App) validateOutput(output string) error {
	_, err := a.FS.Stat(output)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("output directory: %w", err)
	}
	entries, err := afero.ReadDir(a.FS, output)
	if err != nil {
		return fmt.Errorf("output directory: %w", err)
	}
	if len(entries) != 0 {
		return errors.New("output directory must be empty; existing work will not be overwritten")
	}
	return nil
}

func safeName(name string) bool {
	return fs.ValidPath(name) && name != "." && !strings.ContainsAny(name, "\\:")
}

func sortedNames(assets resource.Assets) []string {
	names := make([]string, 0, len(assets))
	for name := range assets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func addAssets(dst resource.Assets, prefix string, src resource.Assets) error {
	for name, data := range src {
		if !safeName(name) {
			return fmt.Errorf("invalid resource path: %q", name)
		}
		name = path.Join(prefix, name)
		if previous, exists := dst[name]; exists && !bytes.Equal(previous, data) {
			return fmt.Errorf("conflicting resources at %s; unpack each game version separately", name)
		}
		dst[name] = data
	}
	return nil
}

func (a App) handler(name string, byName bool) resource.Handler {
	for _, h := range a.Handlers {
		if byName {
			if h.Name() == name {
				return h
			}
			continue
		}
		for _, pattern := range h.Patterns() {
			if matched, _ := path.Match(strings.ToUpper(pattern), strings.ToUpper(name)); matched {
				return h
			}
		}
	}
	return nil
}

func (a App) unpack(input string) (resource.Assets, error) {
	entries, err := afero.ReadDir(a.FS, input)
	if err != nil {
		return nil, err
	}
	files := resource.Assets{}
	for _, entry := range entries {
		name := entry.Name()
		ext := strings.ToUpper(filepath.Ext(name))
		if entry.IsDir() || (ext != ".DD2" && ext != ".EXE") {
			continue
		}
		data, err := afero.ReadFile(a.FS, filepath.Join(input, name))
		if err != nil {
			return nil, err
		}
		h := a.handler(name, false)
		if h == nil {
			files[path.Join("meta", name)] = data
			continue
		}
		u, err := h.Unpack(name, data)
		if err != nil {
			return nil, fmt.Errorf("unpack %s: %w", name, err)
		}
		manifest, err := json.MarshalIndent(u.Manifest, "", "  ")
		if err != nil {
			return nil, err
		}
		metaPath := path.Join("meta", strings.TrimSuffix(name, filepath.Ext(name)))
		if u.Meta == nil {
			u.Meta = resource.Assets{}
		}
		u.Meta["manifest.json"] = manifest
		if err := addAssets(files, "translatable", u.Translatable); err != nil {
			return nil, err
		}
		if err := addAssets(files, metaPath, u.Meta); err != nil {
			return nil, err
		}
	}
	if len(files) == 0 {
		return nil, errors.New("no game files (.DD2 or .EXE) found")
	}
	return files, nil
}

func (a App) readAssets(root string) (resource.Assets, error) {
	assets := resource.Assets{}
	err := afero.Walk(a.FS, root, func(name string, info fs.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		data, err := afero.ReadFile(a.FS, name)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, name)
		assets[filepath.ToSlash(rel)] = data
		return nil
	})
	return assets, err
}

func (a App) repack(input string) (resource.Assets, error) {
	metaRoot := filepath.Join(input, "meta")
	meta, err := a.readAssets(metaRoot)
	if err != nil {
		return nil, fmt.Errorf("read metadata: %w", err)
	}
	translatable, err := a.readAssets(filepath.Join(input, "translatable"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	files := resource.Assets{}
	for _, name := range sortedNames(meta) {
		if !strings.Contains(name, "/") {
			if err := addAssets(files, "", resource.Assets{name: meta[name]}); err != nil {
				return nil, err
			}
			continue
		}
		if path.Base(name) != "manifest.json" {
			continue
		}
		var manifest resource.Manifest
		if err := json.Unmarshal(meta[name], &manifest); err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		if !safeName(manifest.SourceFile) || path.Base(manifest.SourceFile) != manifest.SourceFile {
			return nil, fmt.Errorf("invalid source filename in %s", name)
		}
		h := a.handler(manifest.Handler, true)
		if h == nil {
			return nil, fmt.Errorf("unknown handler %q in %s", manifest.Handler, name)
		}
		localMeta := resource.Assets{}
		prefix := path.Dir(name) + "/"
		for key, value := range meta {
			if strings.HasPrefix(key, prefix) {
				localMeta[strings.TrimPrefix(key, prefix)] = value
			}
		}
		data, err := h.Repack(manifest, translatable, localMeta)
		if err != nil {
			return nil, fmt.Errorf("repack %s: %w", manifest.SourceFile, err)
		}
		if err := addAssets(files, "", resource.Assets{manifest.SourceFile: data}); err != nil {
			return nil, err
		}
	}
	if len(files) == 0 {
		return nil, errors.New("no manifests or original files found in meta")
	}
	return files, nil
}
