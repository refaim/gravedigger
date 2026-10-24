package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunUsage(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"--version"}, nil, {"unpack", "does-not-exist", "unused-output"}} {
		var out, stderr bytes.Buffer
		code := run(args, &out, &stderr)
		if len(args) == 1 {
			if code != 0 || !strings.Contains(out.String(), "gravedigger") || stderr.Len() != 0 {
				t.Fatalf("%v: %d %s %s", args, code, out.String(), stderr.String())
			}
		} else if code != 1 || !strings.HasPrefix(stderr.String(), "Error:") {
			t.Fatalf("%v: %d %s", args, code, stderr.String())
		}
	}
}

func TestVersion(t *testing.T) {
	var out, stderr bytes.Buffer
	if code := run([]string{"--version"}, &out, &stderr); code != 0 || out.String() != "gravedigger 3.1.0\n" || stderr.Len() != 0 {
		t.Fatalf("version = %q, code = %d, stderr = %q", out.String(), code, stderr.String())
	}
}

func TestMainDelegatesExitStatus(t *testing.T) {
	previousArgs, previousExit := os.Args, exit
	t.Cleanup(func() { os.Args, exit = previousArgs, previousExit })
	os.Args = []string{"gravedigger", "--version"}
	code := -1
	exit = func(status int) { code = status }
	main()
	if code != 0 {
		t.Fatal(code)
	}
}

func TestCLIOriginalGameRoundtrip(t *testing.T) {
	for _, variant := range []string{"softdisk", "retail"} {
		t.Run(variant, func(t *testing.T) {
			input := filepath.Join("..", "..", "game", variant)
			if _, err := os.Stat(input); err != nil {
				t.Fatal("required game corpus missing:", err)
			}
			base := t.TempDir()
			editable, output := filepath.Join(base, "editable"), filepath.Join(base, "output")
			var out, stderr bytes.Buffer
			for _, args := range [][]string{{"unpack", input, editable}, {"repack", editable, output}} {
				if code := run(args, &out, &stderr); code != 0 {
					t.Fatalf("%v: %s", args, stderr.String())
				}
			}
			entries, err := os.ReadDir(input)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				ext := strings.ToUpper(filepath.Ext(entry.Name()))
				if ext != ".EXE" && ext != ".DD2" {
					continue
				}
				want, err := os.ReadFile(filepath.Join(input, entry.Name()))
				if err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(filepath.Join(output, entry.Name()))
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(want, got) {
					t.Fatal("roundtrip changed", entry.Name())
				}
			}
		})
	}
}
