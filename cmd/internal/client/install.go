package client

import (
	"github.com/fuseml/fuseml/cli/kubernetes"
	"github.com/fuseml/fuseml/cli/paas"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var NeededOptions = kubernetes.InstallationOptions{
	{
		Name:        "system_domain",
		Description: "The domain you are planning to use for Fuseml. Should be pointing to the traefik public IP (Leave empty to use a omg.howdoi.website domain).",
		Type:        kubernetes.StringType,
		Default:     "",
		Value:       "",
	},
}

const (
	DefaultOrganization = "workspace"
)

var CmdInstall = &cobra.Command{
	Use:           "install",
	Short:         "install Fuseml in your configured kubernetes cluster",
	Long:          `install Fuseml PaaS in your configured kubernetes cluster`,
	Args:          cobra.ExactArgs(0),
	RunE:          Install,
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	CmdInstall.Flags().BoolP("interactive", "i", false, "Whether to ask the user or not (default not)")

	NeededOptions.AsCobraFlagsFor(CmdInstall)
}

// Install command installs fuseml on a configured cluster
func Install(cmd *cobra.Command, args []string) error {
	install_client, install_cleanup, err := paas.NewInstallClient(cmd.Flags(), nil)
	defer func() {
		if install_cleanup != nil {
			install_cleanup()
		}
	}()

	if err != nil {
		return errors.Wrap(err, "error initializing cli")
	}

	err = install_client.Install(cmd, &NeededOptions)
	if err != nil {
		return errors.Wrap(err, "error installing FuseML")
	}

	return nil
}
