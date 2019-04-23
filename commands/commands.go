package commands

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"

	"github.com/buildpack/pack/style"

	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/logging"
)

//go:generate mockgen -package mocks -destination mocks/pack_client.go github.com/buildpack/pack/commands PackClient
type PackClient interface {
	InspectBuilder(string, bool) (*pack.BuilderInfo, error)
	Rebase(context.Context, pack.RebaseOptions) error
	CreateBuilder(context.Context, pack.CreateBuilderOptions) error
}


type suggestedBuilder struct {
	name  string
	image string
	info string
}

var suggestedBuilders = [][]suggestedBuilder{
	{
		{"Cloud Foundry", "cloudfoundry/cnb:bionic", "small base image with Java & Node.js"},
		{"Cloud Foundry", "cloudfoundry/cnb:cflinuxfs3", "larger base image with Java, Node.js & Python"},
	},
	{
		{"Heroku", "heroku/buildpacks", "heroku-18 base image with official Heroku buildpacks"},
	},
}

func AddHelpFlag(cmd *cobra.Command, commandName string) {
	cmd.Flags().BoolP("help", "h", false, fmt.Sprintf("Help for '%s'", commandName))
}

func logError(logger *logging.Logger, f func(cmd *cobra.Command, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		err := f(cmd, args)
		if err != nil {
			if !IsSoftError(err) {
				logger.Error(err.Error())
			}
			return err
		}
		return nil
	}
}

func multiValueHelp(name string) string {
	return fmt.Sprintf("\nRepeat for each %s in order,\n  or supply once by comma-separated list", name)
}

func createCancellableContext() context.Context {
	signals := make(chan os.Signal)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		<-signals
		cancel()
	}()

	return ctx
}

func suggestSettingBuilder(logger *logging.Logger) {
	logger.Info("Please select a default builder with:\n")
	logger.Info("\tpack set-default-builder <builder image>\n")
	suggestBuilders(logger)
}

func suggestBuilders(logger *logging.Logger) {
	logger.Info("Suggested builders:\n")

	tw := tabwriter.NewWriter(logger.RawWriter(), 10, 10, 5, ' ', tabwriter.TabIndent)
	for _, i := range rand.Perm(len(suggestedBuilders)) {
		builders := suggestedBuilders[i]
		for _, builder := range builders {
			_, _ = tw.Write([]byte(fmt.Sprintf("\t%s:\t%s\t%s\t\n", builder.name, style.Symbol(builder.image), builder.info)))
		}
	}
	_ = tw.Flush()

	logger.Info("")
	logger.Tip("Learn more about a specific builder with:\n")
	logger.Info("\tpack inspect-builder [builder image]")
}