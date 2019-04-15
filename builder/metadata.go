package builder

import (
	"github.com/buildpack/pack/stack"
)

const MetadataLabel = "io.buildpacks.builder.metadata"

type Metadata struct {
	Buildpacks []BuildpackMetadata `json:"buildpacks"`
	Groups     []GroupMetadata     `json:"groups"`
	Stack      stack.Metadata      `json:"stack"`
}

type BuildpackMetadata struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Latest  bool   `json:"latest"`
}

type GroupMetadata struct {
	Buildpacks []BuildpackMetadata `json:"buildpacks"`
}
