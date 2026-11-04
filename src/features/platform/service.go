// Package platform exposes host operating system information to the frontend
// so the interface can adapt its chrome without platform-specific builds.
package platform

import "runtime"

// Service answers questions about the host platform.
type Service struct{}

// NewService returns a platform service.
func NewService() *Service {
	return &Service{}
}

// Name returns the operating system identifier used by Go: darwin, linux or
// windows.
func (service *Service) Name() string {
	return runtime.GOOS
}
