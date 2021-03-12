package client

import (
	"github.com/fuseml/fuseml/cli/paas"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var ()

// CmdApps implements the fuseml app command
var CmdApps = &cobra.Command{
	Use:   "apps",
	Short: "Lists all applications",
	Args:  cobra.ExactArgs(0),
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

		err = client.Apps()
		if err != nil {
			return errors.Wrap(err, "error listing apps")
		}

		return nil
	},
	SilenceErrors: true,
	SilenceUsage:  true,
}
