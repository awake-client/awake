package main

import (
	"embed"
	"log"
	"os"

	"github.com/awake-client/awake/src/app"
)

// The frontend build output is embedded into the binary so the application
// runs as a single executable.
//
//go:embed all:frontend/dist
var frontendAssets embed.FS

func main() {
	// `awake mcp` serves a workspace to coding agents over stdio, without a
	// window.
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		if err := app.RunMCP(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := app.Run(frontendAssets); err != nil {
		log.Fatal(err)
	}
}
