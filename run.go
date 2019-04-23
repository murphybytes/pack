package pack

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/pkg/errors"

	"github.com/buildpack/pack/app"
	"github.com/buildpack/pack/style"
)

type RunOptions struct {
	AppDir     string // defaults to current working directory
	Builder    string // defaults to default builder on the client config
	RunImage   string // defaults to the best mirror from the builder image
	Env        map[string]string
	NoPull     bool
	ClearCache bool
	Buildpacks []string
	Ports      []string
}

func (c *Client) Run(ctx context.Context, opts RunOptions) error {
	appDir, err := c.processAppDir(opts.AppDir)
	if err != nil {
		return errors.Wrapf(err, "invalid app dir '%s'", opts.AppDir)
	}
	sum := sha256.Sum256([]byte(appDir))
	imageName := fmt.Sprintf("pack.local/run/%x", sum[:8])
	err = c.Build(ctx, BuildOptions{
		AppDir:     appDir,
		Builder:    opts.Builder,
		RunImage:   opts.RunImage,
		Env:        opts.Env,
		Image:      imageName,
		NoPull:     opts.NoPull,
		ClearCache: opts.ClearCache,
		Buildpacks: opts.Buildpacks,
	})
	if err != nil {
		return errors.Wrap(err, "build failed")
	}
	appImage := &app.Image{RepoName: imageName, Logger: c.logger}
	c.logger.Verbose(style.Step("RUNNING"))
	return appImage.Run(ctx, c.docker, opts.Ports)
}
