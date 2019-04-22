package testhelpers

import (
	"encoding/json"
	"testing"

	"github.com/buildpack/lifecycle/image/fakes"

	"github.com/buildpack/pack/stack"
	"github.com/buildpack/pack/builder"
)

func NewFakeBuilderImage(t *testing.T, name string, config builder.Config) *fakes.Image {
	fakeBuilderImage := fakes.NewImage(t, name, "", "")
	AssertNil(t, fakeBuilderImage.SetLabel("io.buildpacks.stack.id", config.Stack.ID))
	AssertNil(t, fakeBuilderImage.SetEnv("CNB_USER_ID", "1234"))
	AssertNil(t, fakeBuilderImage.SetEnv("CNB_GROUP_ID", "4321"))
	metadata := builder.Metadata{
		Groups: config.Groups,
		Stack: stack.Metadata{
			RunImage: stack.RunImageMetadata{
				Image:   config.Stack.RunImage,
				Mirrors: config.Stack.RunImageMirrors,
			},
		},
	}
	label, err := json.Marshal(&metadata)
	AssertNil(t, err)
	AssertNil(t, fakeBuilderImage.SetLabel("io.buildpacks.builder.metadata", string(label)))
	return fakeBuilderImage
}
