package client

import (
	"github.com/fuseml/fuseml/cli/kubernetes"
	"github.com/fuseml/fuseml/cli/paas"
	"github.com/fuseml/fuseml/cli/paas/config"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var InstallOptions = kubernetes.InstallationOptions{
	{
		Name:        "system_domain",
		Description: "The domain you are planning to use for FuseML. Should be pointing to the load balancer public IP (Leave empty to use a nip.io domain).",
		Type:        kubernetes.StringType,
		Default:     "",
		Value:       "",
	},
	{
		Name:        "extensions",
		Description: "ML extensions to install together with FuseML",
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

const (
	DefaultOrganization = "workspace"
)

var CmdInstall = &cobra.Command{
	Use:           "install",
	Short:         "install FuseML in your configured kubernetes cluster",
	Long:          `install FuseML in your configured kubernetes cluster`,
	Args:          cobra.ExactArgs(0),
	RunE:          Install,
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	CmdInstall.Flags().BoolP("interactive", "i", false, "Whether to ask the user or not (default not)")

	InstallOptions.AsCobraFlagsFor(CmdInstall)
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

	err = install_client.Install(cmd, &InstallOptions)
	if err != nil {
		return errors.Wrap(err, "error installing FuseML")
	}

	return nil
}
