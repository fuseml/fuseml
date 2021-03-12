package client

import (
	"github.com/fuseml/fuseml/cli/paas"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var ()

// CmdTarget implements the fuseml target command
var CmdTarget = &cobra.Command{
	Use:   "target [org]",
	Short: "Targets an organization in Fuseml.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, cleanup, err := paas.NewFusemlClient(cmd.Flags(), nil)
		defer func() {
			if cleanup != nil {
				cleanup()
			}
		}()

		if err != nil {
			return errors.Wrap(err, "error initializing cli")
		}

		org := ""
		if len(args) > 0 {
			org = args[0]
		}

		err = client.Target(org)
		if err != nil {
			return errors.Wrap(err, "failed to set target")
		}

		return nil
	},
	SilenceErrors: true,
	SilenceUsage:  true,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		app, cleanup, _ := paas.NewFusemlClient(cmd.Flags(), nil)
		defer func() {
			if cleanup != nil {
				cleanup()
			}
		}()

		matches := app.OrgsMatching(toComplete)

		return matches, cobra.ShellCompDirectiveNoFileComp
	},
}
