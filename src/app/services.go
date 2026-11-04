package app

import "github.com/clementherve/awake/src/features/workspace"

// services holds the feature services exposed to the frontend. Dependencies are
// wired here so features never construct each other directly.
type services struct {
	Workspace *workspace.Service
}

func newServices() *services {
	return &services{
		Workspace: workspace.NewService(),
	}
}
