// Command placard is the Placard CLI + HTTP service in one static binary
// (construct-server house style).
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/Einlanzerous/placard/internal/gen"
	"github.com/Einlanzerous/placard/internal/version"
)

func main() {
	// No args defaults to serve — the container entrypoint runs the binary bare.
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	var err error
	switch cmd {
	case "serve":
		err = runServe()
	case "check":
		err = runCheck()
	case "migrate":
		err = runMigrate()
	case "gen":
		dir := "."
		if len(os.Args) > 2 {
			dir = os.Args[2]
		}
		err = gen.Run(dir, log.Printf)
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
	if err != nil {
		log.Fatalf("%s: %v", cmd, err)
	}
}

func usage(w *os.File) {
	fmt.Fprint(w, `placard — canonical home for Construct service marks

usage: placard [command]

  serve       front page + mark mirror + API (the default)
  check       verify every canonical URL once; record if a database is configured
  migrate     apply embedded migrations and exit
  gen [dir]   regenerate derived marks (svg rasters + -dev.png variants)
  version     print build identity
`)
}
