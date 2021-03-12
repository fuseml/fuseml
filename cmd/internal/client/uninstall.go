package client

import (
	"github.com/fuseml/fuseml/cli/paas"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var CmdUninstall = &cobra.Command{
	Use:           "uninstall",
	Short:         "uninstall Fuseml from your configured kubernetes cluster",
	Long:          `uninstall Fuseml PaaS from your configured kubernetes cluster`,
	Args:          cobra.ExactArgs(0),
	RunE:          Uninstall,
	SilenceErrors: true,
	SilenceUsage:  true,
}

// Uninstall command removes fuseml from a configured cluster
func Uninstall(cmd *cobra.Command, args []string) error {
	installClient, _, err := paas.NewInstallClient(cmd.Flags(), nil)
	if err != nil {
		return errors.Wrap(err, "error initializing cli")
	}

	err = installClient.Uninstall(cmd)
	if err != nil {
		return err
	}

	return nil
}
