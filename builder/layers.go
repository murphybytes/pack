package builder

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/lifecycle"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/buildpack"

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

func BuildpackLayer(dest, bpDir string, buildpack *buildpack.Buildpack) (layerTar string, err error) {
	dir := buildpack.Dir

	data, err := buildpackData(dir)
	if err != nil {
		return "", err
	}
	bp := data.BP
	if buildpack.ID != bp.ID {
		return "", fmt.Errorf("buildpack IDs did not match: %s != %s", buildpack.ID, bp.ID)
	}
	if bp.Version == "" {
		return "", fmt.Errorf("buildpack.toml must provide version: %s", filepath.Join(buildpack.Dir, "buildpack.toml"))
	}

	buildpack.Version = bp.Version
	tarFile := filepath.Join(dest, fmt.Sprintf("%s.%s.tar", buildpack.EscapedID(), bp.Version))
	if err := archive.CreateTar(tarFile, dir, filepath.Join("/buildpacks", buildpack.EscapedID(), bp.Version), 0, 0); err != nil {
		return "", err
	}
	return tarFile, err
}

func buildpackData(dir string) (*buildpack.TOML, error) {
	data := &buildpack.TOML{}
	_, err := toml.DecodeFile(filepath.Join(dir, "buildpack.toml"), &data)
	if err != nil {
		return nil, errors.Wrapf(err, "reading buildpack.toml from buildpack: %s", dir)
	}
	return data, nil
}