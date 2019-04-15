package builder

import (
	"github.com/buildpack/pack/stack"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/lifecycle"
	"github.com/buildpack/pack/archive"
)

type order struct {
	Groups []lifecycle.BuildpackGroup `toml:"groups"`
}

func OrderLayer(dest string, groups []lifecycle.BuildpackGroup) (layerTar string, err error) {
	bpDir := filepath.Join(dest, "buildpacks")
	err = os.Mkdir(bpDir, 0755)
	if err != nil {
		return "", err
	}

	orderFile, err := os.OpenFile(filepath.Join(bpDir, "order.toml"), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return "", err
	}
	defer orderFile.Close()
	err = toml.NewEncoder(orderFile).Encode(order{Groups: groups})
	if err != nil {
		return "", err
	}
	layerTar = filepath.Join(dest, "order.tar")
	if err := archive.CreateTar(layerTar, bpDir, "/buildpacks", 0, 0); err != nil {
		return "", err
	}
	return layerTar, nil
}

func StackLayer(dest string, runImage string, mirrors []string) (layerTar string, err error) {
	bpDir := filepath.Join(dest, "buildpacks")
	if err := os.MkdirAll(bpDir, 0755); err != nil {
		return "", err
	}

	stackFile, err := os.OpenFile(filepath.Join(bpDir, "stack.toml"), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return "", err
	}
	defer stackFile.Close()

	content := stack.Metadata{
		RunImage: stack.RunImageMetadata{
			Image:   runImage,
			Mirrors: mirrors,
		},
	}
	if err = toml.NewEncoder(stackFile).Encode(&content); err != nil {
		return "", err
	}

	layerTar = filepath.Join(dest, "stack.tar")
	if err := archive.CreateTar(layerTar, bpDir, "/buildpacks", 0, 0); err != nil {
		return "", err
	}

	return layerTar, nil
}