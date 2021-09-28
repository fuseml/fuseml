package client

import (
	"github.com/fuseml/fuseml/cli/kubernetes"
	"github.com/fuseml/fuseml/cli/paas"
	"github.com/fuseml/fuseml/cli/paas/config"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var extensionsOptions = kubernetes.InstallationOptions{
	{
		Name:        "system_domain",
		Description: "The domain you are using for FuseML. Should be pointing to the load balancer public IP (Leave empty to use a nip.io domain).",
		Type:        kubernetes.StringType,
		Default:     "",
		Value:       "",
	},
	{
		Name:        "add",
		Description: "FuseML extension to install into existing deployment",
		Type:        kubernetes.ListType,
		Default:     []string{},
		Value:       []string{},
	},
	{
		Name:        "remove",
		Description: "FuseML extension to remove from deployment",
		Type:        kubernetes.ListType,
		Default:     []string{},
		Value:       []string{},
	},
	{
		Name:        "with_dependencies",
		Description: "When removing an extension, remove also all its required extensions",
		Type:        kubernetes.BooleanType,
		Default:     false,
		Value:       true,
	},
	{
		Name:        "list",
		Description: "List installed FuseML extensions",
		Type:        kubernetes.BooleanType,
		Default:     false,
		Value:       true,
	},
	{
		Name:        "extensions_repository",
		Description: "Path to extensions repository. Could be local directory or URL",
		Type:        kubernetes.StringType,
		Default:     config.DefaultExtensionsLocation(),
		Value:       "",
	},
}

func init() {
	extensionsOptions.AsCobraFlagsFor(CmdExtensions)
}

var CmdExtensions = &cobra.Command{
	Use:           "extensions",
	Short:         "Add or remove FuseML extensions",
	Long:          `Add or remove FuseML extensions`,
	Args:          cobra.ExactArgs(0),
	RunE:          Extensions,
	SilenceErrors: true,
	SilenceUsage:  true,
}

func Extensions(cmd *cobra.Command, args []string) error {
	install_client, install_cleanup, err := paas.NewInstallClient(cmd.Flags(), nil)
	defer func() {
		if install_cleanup != nil {
			install_cleanup()
		}
	}()

	if err != nil {
		return errors.Wrap(err, "error initializing cli")
	}

	err = install_client.Extensions(cmd, &extensionsOptions)
	if err != nil {
		return errors.Wrap(err, "error when handling FuseML extensions")
	}

	return nil
}
