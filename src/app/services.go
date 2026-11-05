package app

import (
	"github.com/clementherve/awake/src/features/collections"
	"github.com/clementherve/awake/src/features/environments"
	"github.com/clementherve/awake/src/features/platform"
	"github.com/clementherve/awake/src/features/workspace"
)

// services holds the feature services exposed to the frontend. Dependencies are
// wired here so features never construct each other directly.
type services struct {
	Collections  *collections.Service
	Environments *environments.Service
	Platform     *platform.Service
	Workspace    *workspace.Service
}

func newServices() *services {
	return &services{
		Collections:  collections.NewService(),
		Environments: environments.NewService(),
		Platform:     platform.NewService(),
		Workspace:    workspace.NewService(),
	}
}
