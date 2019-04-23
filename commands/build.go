package commands

import (
	"io/ioutil"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpack/pack/config"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
)

type BuildFlags struct {
	AppDir     string
	Builder    string
	RunImage   string
	Env        []string
	EnvFile    string
	Publish    bool
	NoPull     bool
	ClearCache bool
	Buildpacks []string
}

func Build(logger *logging.Logger, config *config.Config, packClient *pack.Client) *cobra.Command {
	var flags BuildFlags
	ctx := createCancellableContext()

	cmd := &cobra.Command{
		Use:   "build <image-name>",
		Args:  cobra.ExactArgs(1),
		Short: "Generate app image from source code",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			imageName := args[0]
			if config.DefaultBuilder == "" && flags.Builder == "" {
				suggestSettingBuilder(logger)
				return MakeSoftError()
			}
			env, err := parseEnv(flags.EnvFile, flags.Env)
			if err != nil {
				return err
			}
			if err := packClient.Build(ctx, pack.BuildOptions{
				AppDir:     flags.AppDir,
				Builder:    flags.Builder,
				RunImage:   flags.RunImage,
				Env:        env,
				Image:      imageName,
				Publish:    flags.Publish,
				NoPull:     flags.NoPull,
				ClearCache: flags.ClearCache,
				Buildpacks: flags.Buildpacks,
			}); err != nil {
				return err
			}
			logger.Info("Successfully built image %s", style.Symbol(imageName))
			return nil
		}),
	}
	buildCommandFlags(cmd, &flags)
	cmd.Flags().BoolVar(&flags.Publish, "publish", false, "Publish to registry")
	AddHelpFlag(cmd, "build")
	return cmd
}

func buildCommandFlags(cmd *cobra.Command, buildFlags *BuildFlags) {
	cmd.Flags().StringVarP(&buildFlags.AppDir, "path", "p", "", "Path to app dir (defaults to current working directory)")
	cmd.Flags().StringVar(&buildFlags.Builder, "builder", "", "Builder (defaults to builder configured by 'set-default-builder')")
	cmd.Flags().StringVar(&buildFlags.RunImage, "run-image", "", "Run image (defaults to default stack's run image)")
	cmd.Flags().StringArrayVarP(&buildFlags.Env, "env", "e", []string{}, "Build-time environment variable, in the form 'VAR=VALUE' or 'VAR'.\nWhen using latter value-less form, value will be taken from current\n  environment at the time this command is executed.\nThis flag may be specified multiple times and will override\n  individual values defined by --env-file.")
	cmd.Flags().StringVar(&buildFlags.EnvFile, "env-file", "", "Build-time environment variables file\nOne variable per line, of the form 'VAR=VALUE' or 'VAR'\nWhen using latter value-less form, value will be taken from current\n  environment at the time this command is executed")
	cmd.Flags().BoolVar(&buildFlags.NoPull, "no-pull", false, "Skip pulling builder and run images before use")
	cmd.Flags().BoolVar(&buildFlags.ClearCache, "clear-cache", false, "Clear image's associated cache before building")
	cmd.Flags().StringSliceVar(&buildFlags.Buildpacks, "buildpack", nil, "Buildpack ID or path to a buildpack directory"+multiValueHelp("buildpack"))
}

func parseEnv(envFile string, envVars []string) (map[string]string, error) {
	env := map[string]string{}
	if envFile != "" {
		var err error
		env, err = parseEnvFile(envFile)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse env file '%s'", envFile)
		}
	}
	for _, envVar := range envVars {
		env = addEnvVar(env, envVar)
	}
	return env, nil
}

func parseEnvFile(filename string) (map[string]string, error) {
	out := make(map[string]string, 0)
	f, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, errors.Wrapf(err, "open %s", filename)
	}
	for _, line := range strings.Split(string(f), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = addEnvVar(out, line)
	}
	return out, nil
}

func addEnvVar(env map[string]string, item string) map[string]string {
	arr := strings.SplitN(item, "=", 2)
	if len(arr) > 1 {
		env[arr[0]] = arr[1]
	} else {
		env[arr[0]] = os.Getenv(arr[0])
	}
	return env
}