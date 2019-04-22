package pack

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/buildpack/lifecycle/image"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/build"
	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/buildpack"
	"github.com/buildpack/pack/stack"
	"github.com/buildpack/pack/style"
)

type BuildOptions struct {
	AppDir     string // defaults to current working directory
	Builder    string // defaults to default builder on the client config
	RunImage   string // defaults to the best mirror from the builder image
	Env        map[string]string
	Image      string // required
	Publish    bool
	NoPull     bool
	ClearCache bool
	Buildpacks []string
}

func (c *Client) Build(ctx context.Context, opts BuildOptions) error {
	imageRef, err := c.processTagReference(opts.Image)
	if err != nil {
		return errors.Wrapf(err, "invalid image name '%s'", opts.Image)
	}
	appDir, err := c.processAppDir(opts.AppDir)
	if err != nil {
		return errors.Wrapf(err, "invalid app dir '%s'", opts.AppDir)
	}

	builderRef, err := c.processBuilderName(opts.Builder)
	if err != nil {
		return errors.Wrapf(err, "invalid builder '%s'", opts.Builder)
	}

	rawBuilderImage, err := c.imageFetcher.Fetch(ctx, builderRef.Name(), true, !opts.NoPull)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch builder image '%s'", builderRef.Name())
	}

	builderImage, err := c.processBuilderImage(rawBuilderImage)
	if err != nil {
		return errors.Wrapf(err, "invalid builder '%s'", opts.Builder)
	}

	runImageRef, err := c.processRunImageName(opts.RunImage, imageRef.Context().RegistryStr(), builderImage.GetStackInfo())
	if err != nil {
		return errors.Wrap(err, "invalid run-image")
	}

	if _, err := c.validateRunImage(ctx, runImageRef.Name(), opts.NoPull, opts.Publish, builderImage.StackID); err != nil {
		return errors.Wrapf(err, "invalid run-image '%s'", runImageRef.Name())
	}
	extraBuildpacks, group, err := c.processBuildpacks(opts.Buildpacks)
	if err != nil {
		return errors.Wrap(err, "invalid buildpack")
	}

	ephemeralBuilder, err := c.createEphemeralBuilder(rawBuilderImage, opts.Env, group, extraBuildpacks)
	if err != nil {
		return errors.Wrap(err, "failed to create ephemeral builder image")
	}
	defer rawBuilderImage.Delete()

	return c.lifecycle.Execute(ctx, build.LifecycleOptions{
		AppDir:      appDir,
		ImageRef:    imageRef,
		Builder:     ephemeralBuilder,
		RunImageRef: runImageRef,
		ClearCache:  opts.ClearCache,
		Publish:     opts.Publish,
	})
}

func (c *Client) processBuilderName(builderName string) (name.Reference, error) {
	if builderName == "" {
		if c.config.DefaultBuilder != "" {
			builderName = c.config.DefaultBuilder
		} else {
			return nil, errors.New("builder is a required parameter if the client has no default builder")
		}
	}
	return name.ParseReference(builderName, name.WeakValidation);
}

func (c *Client) processBuilderImage(img image.Image) (*builder.Builder, error) {
	builder, err := builder.GetBuilder(img)
	if err != nil {
		return nil, err
	}
	if builder.GetStackInfo().RunImage.Image == "" {
		return nil, errors.New("builder metadata is missing runImage")
	}
	return builder, nil
}

func (c *Client) processRunImageName(runImage, targetRegistry string, builderStackInfo stack.Metadata) (name.Reference, error) {
	if runImage != "" {
		return name.ParseReference(runImage, name.WeakValidation)
	}
	localMirrors := []string{}
	localRunImageConfig := c.config.GetRunImage(builderStackInfo.RunImage.Image)
	if localRunImageConfig != nil {
		localMirrors = localRunImageConfig.Mirrors
	}
	runImageName := builderStackInfo.GetBestMirror(targetRegistry, localMirrors)
	return name.ParseReference(runImageName, name.WeakValidation)
}

func (c *Client) validateRunImage(context context.Context, name string, noPull bool, publish bool, expectedStack string) (image.Image, error) {
	fmt.Println("fetching run image", name)
	img, err := c.imageFetcher.Fetch(context, name, !publish, !noPull)
	if err != nil {
		return nil, err
	}
	stackID, err := img.Label("io.buildpacks.stack.id")
	if err != nil {
		return nil, err
	}
	if stackID != expectedStack {
		return nil, fmt.Errorf("run-image stack id '%s' does not match builder stack '%s'", stackID, expectedStack)
	}
	return img, nil
}

func (c *Client) processTagReference(imageName string) (name.Reference, error) {
	if imageName == "" {
		return nil, errors.New("image name is a required parameter")
	}
	if _, err := name.ParseReference(imageName, name.WeakValidation); err != nil {
		return nil, err
	}
	ref, err := name.NewTag(imageName, name.WeakValidation);
	if err != nil {
		return nil, fmt.Errorf("'%s' is not a tag reference", imageName)
	}

	return ref, nil
}

func (c *Client) processAppDir(appDir string) (string, error) {
	if appDir == "" {
		return os.Getwd()
	}
	if fi, err := os.Stat(appDir); err != nil {
		return "", err
	} else if !fi.IsDir() {
		return "", fmt.Errorf("'%s' is not a directory", appDir)
	}
	return appDir, nil
}

func (c *Client) processBuildpacks(buildpacks []string) ([]buildpack.Buildpack, builder.GroupMetadata, error) {
	group := builder.GroupMetadata{Buildpacks: []builder.GroupBuildpack{}}
	bps := []buildpack.Buildpack{}
	for _, bp := range buildpacks {
		if isLocalBuildpack(bp) {
			if runtime.GOOS == "windows" {
				return nil, builder.GroupMetadata{}, fmt.Errorf("directory buildpacks are not implemented on windows")
			}
			fetchedBP, err := c.buildpackFetcher.FetchBuildpack(bp)
			if err != nil {
				return nil, builder.GroupMetadata{}, errors.Wrapf(err, "failed to fetch buildpack from uri '%s'", bp)
			}
			bps = append(bps, fetchedBP)
			group.Buildpacks = append(group.Buildpacks, builder.GroupBuildpack{ID: fetchedBP.ID, Version: fetchedBP.Version})
		} else {
			id, version := c.parseBuildpack(bp)
			group.Buildpacks = append(group.Buildpacks, builder.GroupBuildpack{ID: id, Version: version})
		}
	}
	return bps, group, nil
}

func isLocalBuildpack(path string) bool {
	if _, err := os.Stat(filepath.Join(path, "buildpack.toml")); !os.IsNotExist(err) {
		return true
	}
	return false
}

func (c *Client) parseBuildpack(bp string) (string, string) {
	parts := strings.Split(bp, "@")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	c.logger.Verbose("No version for %s buildpack provided, will use %s", style.Symbol(parts[0]), style.Symbol(parts[0]+"@latest"))
	return parts[0], "latest"
}

func (c *Client) createEphemeralBuilder(rawBuilderImage image.Image, env map[string]string, group builder.GroupMetadata, buildpacks []buildpack.Buildpack) (*builder.Builder, error) {
	bldr, err := builder.New(rawBuilderImage, fmt.Sprintf("pack.local/builder/%x:latest", randString(10)))
	if err != nil {
		return nil, err
	}
	bldr.SetEnv(env)
	if len(group.Buildpacks) > 0 {
		bldr.SetOrder([]builder.GroupMetadata{
			group,
		})
	}
	for _, bp := range buildpacks {
		if err := bldr.AddBuildpack(bp); err != nil {
			return nil, err
		}
	}
	if err := bldr.Save(); err != nil {
		return nil, err
	}
	return bldr, nil
}

func randString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a' + byte(rand.Intn(26))
	}
	return string(b)
}
