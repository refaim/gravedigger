package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/afero"

	"github.com/refaim/gravedigger/internal/app"
	"github.com/refaim/gravedigger/internal/exe"
	"github.com/refaim/gravedigger/internal/graphics"
	"github.com/refaim/gravedigger/internal/resource"
)

var exit = os.Exit

func main() {
	exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	command := app.App{
		FS: afero.NewOsFs(), Out: stdout,
		Handlers: []resource.Handler{graphics.PicHandler{}, graphics.SpriteHandler{}, graphics.IntroHandler{}, exe.ExeTextHandler{}},
	}
	if err := command.Run(args); err != nil {
		// A broken stderr has no fallback destination; the exit status still
		// reports failure even if the diagnostic cannot be written.
		_, _ = fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	return 0
}
