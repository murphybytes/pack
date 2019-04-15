package buildpack

import (
	"fmt"
	"github.com/buildpack/pack/archive"
	"github.com/pkg/errors"
	"path/filepath"
	"strings"
)

type Buildpack struct {
	ID      string `toml:"id"`
	URI     string `toml:"uri"`
	Latest  bool   `toml:"latest"`
	Dir     string
	Version string
}

func (b *Buildpack) EscapedID() string {
	return strings.Replace(b.ID, "/", "_", -1)
}

func (b *Buildpack) MakeLayer(dest string) (string, error) {
	tarFile := filepath.Join(dest, fmt.Sprintf("%s.%s.tar", b.EscapedID(), b.Version))
	if err := archive.CreateTar(tarFile, b.Dir, filepath.Join("/buildpacks", b.EscapedID(), b.Version), 0, 0); err != nil {
		return "", errors.Wrapf(err, "failed to make layer tar for buildpack %s:%s", b.ID, b.Version)
	}
	return tarFile, nil
}