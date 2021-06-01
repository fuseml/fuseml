package client

import (
	"github.com/fuseml/fuseml/cli/kubernetes"
	"github.com/fuseml/fuseml/cli/paas"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var upgradeOptions = kubernetes.InstallationOptions{
	{
		Name:        "system_domain",
		Description: "The domain you are planning to use for FuseML. Should be pointing to the load balancer public IP (Leave empty to use a omg.howdoi.website domain).",
		Type:        kubernetes.StringType,
		Default:     "",
		Value:       "",
	},
}

func init() {
	upgradeOptions.AsCobraFlagsFor(CmdUpgrade)
}

var CmdUpgrade = &cobra.Command{
	Use:           "upgrade",
	Short:         "upgrade FuseML in your configured kubernetes cluster",
	Long:          `upgrade FuseML in your configured kubernetes cluster`,
	Args:          cobra.ExactArgs(0),
	RunE:          Upgrade,
	SilenceErrors: true,
	SilenceUsage:  true,
}

func Upgrade(cmd *cobra.Command, args []string) error {
	install_client, install_cleanup, err := paas.NewInstallClient(cmd.Flags(), nil)
	defer func() {
		if install_cleanup != nil {
			install_cleanup()
		}
	}()

	if err != nil {
		return errors.Wrap(err, "error initializing cli")
	}

	err = install_client.Upgrade(cmd, &upgradeOptions)
	if err != nil {
		return errors.Wrap(err, "error upgrading FuseML")
	}

	return nil
}
