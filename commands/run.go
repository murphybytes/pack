package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
)


func Run(logger *logging.Logger, config *config.Config, packClient *pack.Client) *cobra.Command {
	var flags BuildFlags
	ctx := createCancellableContext()

	cmd := &cobra.Command{
		Use:   "build <image-name>",
		Args:  cobra.ExactArgs(1),
		Short: "Generate app image from source code",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			if config.DefaultBuilder == "" && flags.Builder == "" {
				suggestSettingBuilder(logger)
				return MakeSoftError()
			}
			env, err := parseEnv(flags.EnvFile, flags.Env)
			if err != nil {
				return err
			}
			return packClient.Run(ctx, pack.RunOptions{
				AppDir:     flags.AppDir,
				Builder:    flags.Builder,
				RunImage:   flags.RunImage,
				Env:        env,
				NoPull:     flags.NoPull,
				ClearCache: flags.ClearCache,
				Buildpacks: flags.Buildpacks,
			})
		}),
	}
	buildCommandFlags(cmd, flags)
	AddHelpFlag(cmd, "build")
	return cmd
}