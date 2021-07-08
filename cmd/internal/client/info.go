package client

import (
	"github.com/fuseml/fuseml/cli/paas"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var ()

// CmdInfo implements the fuseml info command
var CmdInfo = &cobra.Command{
	Use:   "info",
	Short: "Shows information about the Fuseml environment",
	Long:  `Shows status and version for Kubernetes, Gitea, Tekton.`,
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

		err = client.Info()
		if err != nil {
			return errors.Wrap(err, "error retrieving Fuseml environment information")
		}

		return nil
	},
	SilenceErrors: true,
	SilenceUsage:  true,
}
