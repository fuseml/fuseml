//+build wireinject

package paas

import (
	"github.com/fuseml/fuseml/cli/kubernetes"
	kubeconfig "github.com/fuseml/fuseml/cli/kubernetes/config"
	"github.com/fuseml/fuseml/cli/paas/config"
	"github.com/fuseml/fuseml/cli/paas/gitea"
	"github.com/fuseml/fuseml/cli/paas/ui"
	"github.com/google/wire"
	"github.com/spf13/pflag"
)

// NewCarrierClient creates the Carrier Client
func NewCarrierClient(flags *pflag.FlagSet, configOverrides func(*config.Config)) (*CarrierClient, func(), error) {
	wire.Build(
		wire.Struct(new(CarrierClient), "*"),
		config.Load,
		ui.NewUI,
		gitea.NewGiteaClient,
		gitea.NewResolver,
		kubernetes.NewClusterFromClient,
		kubeconfig.KubeConfig,
		kubeconfig.NewClientLogger,
	)

	return &CarrierClient{}, func() {}, nil
}

// NewInstallClient creates the Carrier Client for installation
func NewInstallClient(flags *pflag.FlagSet, configOverrides func(*config.Config)) (*InstallClient, func(), error) {
	wire.Build(
		wire.Struct(new(InstallClient), "*"),
		config.Load,
		ui.NewUI,
		kubernetes.NewClusterFromClient,
		kubeconfig.KubeConfig,
		kubeconfig.NewInstallClientLogger,
	)

	return &InstallClient{}, func() {}, nil
}
