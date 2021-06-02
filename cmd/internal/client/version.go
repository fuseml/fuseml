package client

import (
	"github.com/fuseml/fuseml/cli/paas"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var ()

// CmdVersion implements the fuseml version command
var CmdVersion = &cobra.Command{
	Use:   "version",
	Short: "Shows version information about the installer",
	Long:  `Shows the version, git commit, build time and other release information about the installer.`,
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

		err = client.Version()
		if err != nil {
			return errors.Wrap(err, "error retrieving Fuseml environment information")
		}

		return nil
	},
	SilenceErrors: true,
	SilenceUsage:  true,
}
