package client

import (
	"github.com/fuseml/fuseml/cli/kubernetes"
	"github.com/fuseml/fuseml/cli/paas"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var uninstallOptions = kubernetes.InstallationOptions{
	{
		Name:        "extensions",
		Description: "ML extensions to uninstall when uninstalling FuseML",
		Type:        kubernetes.ListType,
		Default:     []string{},
		Value:       []string{},
	},
}

var CmdUninstall = &cobra.Command{
	Use:           "uninstall",
	Short:         "uninstall Fuseml from your configured kubernetes cluster",
	Long:          `uninstall Fuseml PaaS from your configured kubernetes cluster`,
	Args:          cobra.ExactArgs(0),
	RunE:          Uninstall,
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	uninstallOptions.AsCobraFlagsFor(CmdUninstall)
}

// Uninstall command removes fuseml from a configured cluster
func Uninstall(cmd *cobra.Command, args []string) error {
	installClient, _, err := paas.NewInstallClient(cmd.Flags(), nil)
	if err != nil {
		return errors.Wrap(err, "error initializing cli")
	}

	err = installClient.Uninstall(cmd, &uninstallOptions)
	if err != nil {
		return err
	}

	return nil
}
