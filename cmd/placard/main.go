// Command placard is the Placard CLI + HTTP service in one static binary
// (construct-server house style). Subcommands land with their tickets; this
// entrypoint grows a case per command, never a second binary.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/Einlanzerous/placard/internal/gen"
	"github.com/Einlanzerous/placard/internal/version"
)

func main() {
	cmd := "help"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "gen":
		dir := "."
		if len(os.Args) > 2 {
			dir = os.Args[2]
		}
		if err := gen.Run(dir, log.Printf); err != nil {
			log.Fatalf("gen: %v", err)
		}
	case "version":
		fmt.Printf("placard %s", version.Resolved())
		if version.Commit != "" {
			fmt.Printf(" (%s)", version.Commit)
		}
		fmt.Println()
	case "help", "-h", "--help":
		usage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "placard: unknown command %q\n\n", cmd)
		usage(os.Stderr)
		os.Exit(2)
	}
}

func usage(w *os.File) {
	fmt.Fprint(w, `placard — canonical home for Construct service marks

usage: placard <command>

  gen [dir]   regenerate derived marks (svg rasters + -dev.png variants)
  version     print build identity
`)
}
