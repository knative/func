package lifecycle

import (
	"github.com/buildpacks/imgutil"

	"github.com/buildpacks/lifecycle/buildpack"
)

//go:generate mockgen -package testmock -destination testmock/cache_handler.go github.com/buildpacks/lifecycle CacheHandler
type CacheHandler interface {
	InitCache(imageRef, dir string) (Cache, error)
}

//go:generate mockgen -package testmock -destination testmock/registry_handler.go github.com/buildpacks/lifecycle RegistryHandler
type ConfigHandler interface {
	ReadGroup(path string) ([]buildpack.GroupBuildpack, error)
}

//go:generate mockgen -package testmock -destination testmock/image_handler.go github.com/buildpacks/lifecycle ImageHandler
type ImageHandler interface {
	InitImage(imageRef string) (imgutil.Image, error)
	Docker() bool
}

//go:generate mockgen -package testmock -destination testmock/registry_handler.go github.com/buildpacks/lifecycle RegistryHandler
type RegistryHandler interface {
	EnsureReadAccess(imageRefs ...string) error
	EnsureWriteAccess(imageRefs ...string) error
}
