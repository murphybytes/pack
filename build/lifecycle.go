package build

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/buildpack/lifecycle/image"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/buildpack"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
)

type Lifecycle struct {
	Builder      *builder.Builder
	Logger       *logging.Logger
	Docker       *client.Client
	LayersVolume string
	AppVolume    string
	appDir       string
	appOnce      *sync.Once
}

type LifecycleConfig struct {
	BuilderImage string
	Logger       *logging.Logger
	Env          map[string]string
	Buildpacks   []string
	AppDir       string
	BPFetcher    *buildpack.Fetcher
}

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

func NewLifecycle(c LifecycleConfig) (*Lifecycle, error) {
	client, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	if err != nil {
		return nil, err
	}

	factory, err := image.NewFactory()
	if err != nil {
		return nil, err
	}

	img, err := factory.NewLocal(c.BuilderImage)
	if err != nil {
		return nil, err
	}

	bldr, err := builder.New(img, fmt.Sprintf("pack.local/builder/%x", randString(10)))
	if err != nil {
		return nil, err
	}

	bldr.SetEnv(c.Env)

	if len(c.Buildpacks) != 0 {
		var ephemeralGroup []builder.GroupBuildpack
		for _, bp := range c.Buildpacks {
			var gb builder.GroupBuildpack
			if isLocalBuildpack(bp) {
				if runtime.GOOS == "windows" {
					return nil, fmt.Errorf("directory buildpacks are not implemented on windows")
				}

				b, err := c.BPFetcher.FetchBuildpack(bp)
				if err != nil {
					return nil, err
				}

				if err := bldr.AddBuildpack(b); err != nil {
					return nil, err
				}

				gb = builder.GroupBuildpack{ID: b.ID, Version: b.Version}
			} else {
				id, version := c.parseBuildpack(bp)

				b, ok := bldr.GetBuildpack(id, version)
				if !ok {
					return nil, fmt.Errorf("buildpack '%s@%s' does not exist in builder '%s'", id, version, bldr.Name())
				}

				gb = builder.GroupBuildpack{ID: b.ID, Version: b.Version}
			}
			ephemeralGroup = append(ephemeralGroup, gb)
		}
		bldr.SetOrder([]builder.GroupMetadata{{Buildpacks: ephemeralGroup}})
	}

	if err := bldr.Save(); err != nil {
		return nil, err
	}

	return &Lifecycle{
		Builder:      bldr,
		Logger:       c.Logger,
		Docker:       client,
		LayersVolume: "pack-layers-" + randString(10),
		AppVolume:    "pack-app-" + randString(10),
		appDir:       c.AppDir,
		appOnce:      &sync.Once{},
	}, nil
}

func (l *Lifecycle) Cleanup() error {
	var reterr error
	if _, err := l.Docker.ImageRemove(context.Background(), l.Builder.Name(), types.ImageRemoveOptions{}); err != nil {
		reterr = errors.Wrapf(err, "failed to clean up builder image %s", l.Builder.Name())
	}
	if err := l.Docker.VolumeRemove(context.Background(), l.LayersVolume, true); err != nil {
		reterr = errors.Wrapf(err, "failed to clean up layers volume %s", l.LayersVolume)
	}
	if err := l.Docker.VolumeRemove(context.Background(), l.AppVolume, true); err != nil {
		reterr = errors.Wrapf(err, "failed to clean up app volume %s", l.AppVolume)
	}
	return reterr
}

func (c *LifecycleConfig) parseBuildpack(bp string) (string, string) {
	parts := strings.Split(bp, "@")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	c.Logger.Verbose("No version for %s buildpack provided, will use %s", style.Symbol(parts[0]), style.Symbol(parts[0]+"@latest"))
	return parts[0], "latest"
}

func isLocalBuildpack(path string) bool {
	if _, err := os.Stat(filepath.Join(path, "buildpack.toml")); !os.IsNotExist(err) {
		return true
	}
	return false
}

func randString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a' + byte(rand.Intn(26))
	}
	return string(b)
}
