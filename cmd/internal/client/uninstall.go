package client

import (
	"github.com/fuseml/fuseml/cli/kubernetes"
	"github.com/fuseml/fuseml/cli/paas"
	"github.com/fuseml/fuseml/cli/paas/config"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var uninstallOptions = kubernetes.InstallationOptions{
	{
		Name:        "system_domain",
		Description: "The domain used by FuseML. Should be pointing to the load balancer public IP (Leave empty to use a nip.io domain).",
		Type:        kubernetes.StringType,
		Default:     "",
		Value:       "",
	},
	{
		Name:        "extensions",
		Description: "ML extensions to uninstall when uninstalling FuseML",
		Type:        kubernetes.ListType,
		Default:     []string{},
		Value:       []string{},
	},
	{
		Name:        "extension_repository",
		Description: "Path to extensions repository. Could be local directory or URL",
		Type:        kubernetes.StringType,
		Default:     config.DefaultExtensionsLocation(),
		Value:       "",
	},
}

var CmdUninstall = &cobra.Command{
	Use:           "uninstall",
	Short:         "Uninstall Fuseml from your configured kubernetes cluster",
	Long:          `Uninstall Fuseml PaaS from your configured kubernetes cluster`,
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
