package build

import (
	"github.com/google/go-containerregistry/pkg/name"

	"github.com/buildpack/pack/builder"

	"github.com/buildpack/pack/cache"

	"github.com/buildpack/pack/style"

	"context"

	"github.com/pkg/errors"
)

type LifecycleOptions struct {
	AppDir      string
	ImageRef    name.Reference
	Builder     *builder.Builder
	RunImageRef name.Reference
	ClearCache  bool
	Publish     bool
}

func (l *Lifecycle) Execute(ctx context.Context, opts LifecycleOptions) error {
	cacheImage := cache.New(opts.ImageRef, l.Docker)
	if opts.ClearCache {
		if err := cacheImage.Clear(ctx); err != nil {
			return errors.Wrap(err, "clearing cache")
		}
		l.Logger.Verbose("Cache image %s cleared", style.Symbol(cacheImage.Image()))
	}
	defer l.Cleanup()

	l.Logger.Verbose(style.Step("DETECTING"))
	if err := l.Detect(ctx); err != nil {
		return err
	}

	l.Logger.Verbose(style.Step("RESTORING"))
	if opts.ClearCache {
		l.Logger.Verbose("Skipping 'restore' due to clearing cache")
	} else if err := l.Restore(ctx, cacheImage.Image()); err != nil {
		return err
	}

	l.Logger.Verbose(style.Step("ANALYZING"))
	if opts.ClearCache {
		l.Logger.Verbose("Skipping 'analyze' due to clearing cache")
	} else {
		if err := l.Analyze(ctx, opts.ImageRef.Name(), opts.Publish); err != nil {
			return err
		}
	}

	l.Logger.Verbose(style.Step("BUILDING"))
	if err := l.Build(ctx); err != nil {
		return err
	}

	l.Logger.Verbose(style.Step("EXPORTING"))
	if err := l.Export(ctx, opts.ImageRef.Name(), opts.RunImageRef.Name(), opts.Publish); err != nil {
		return err
	}

	l.Logger.Verbose(style.Step("CACHING"))
	if err := l.Cache(ctx, cacheImage.Image()); err != nil {
		return err
	}
	return nil
}
