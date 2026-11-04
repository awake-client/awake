package main

import (
	"embed"
	"log"

	"github.com/clementherve/awake/src/app"
)

// The frontend build output is embedded into the binary so the application
// runs as a single executable.
//
//go:embed all:frontend/dist
var frontendAssets embed.FS

func main() {
	if err := app.Run(frontendAssets); err != nil {
		log.Fatal(err)
	}
}
