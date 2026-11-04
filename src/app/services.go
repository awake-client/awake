package app

import (
	"github.com/clementherve/awake/src/features/platform"
	"github.com/clementherve/awake/src/features/workspace"
)

// services holds the feature services exposed to the frontend. Dependencies are
// wired here so features never construct each other directly.
type services struct {
	Platform  *platform.Service
	Workspace *workspace.Service
}

func newServices() *services {
	return &services{
		Platform:  platform.NewService(),
		Workspace: workspace.NewService(),
	}
}
