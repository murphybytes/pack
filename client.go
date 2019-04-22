package pack

import (
	"github.com/docker/docker/client"

	"github.com/buildpack/pack/buildpack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/image"
	"github.com/buildpack/pack/logging"
)

type Client struct {
	config           *config.Config
	logger           *logging.Logger
	imageFetcher     ImageFetcher
	buildpackFetcher BuildpackFetcher
	lifecycle        Lifecycle
}

func NewClient(config *config.Config, logger *logging.Logger, imageFetcher ImageFetcher, lifecycle Lifecycle, buildpackFetcher BuildpackFetcher) *Client {
	return &Client{
		config:           config,
		logger:           logger,
		imageFetcher:     imageFetcher,
		buildpackFetcher: buildpackFetcher,
		lifecycle:        lifecycle,
	}
}

func DefaultClient(config *config.Config, logger *logging.Logger) (*Client, error) {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	if err != nil {
		return nil, err
	}

	imageFetcher, err := image.NewFetcher(logger, dockerClient)
	if err != nil {
		return nil, err
	}

	buildpackFetcher := buildpack.NewFetcher(logger, config.Path())

	return &Client{
		config:           config,
		logger:           logger,
		imageFetcher:     imageFetcher,
		buildpackFetcher: buildpackFetcher,
	}, nil
}
