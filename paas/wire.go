//+build wireinject

package paas

import (
	"github.com/fuseml/fuseml/cli/kubernetes"
	kubeconfig "github.com/fuseml/fuseml/cli/kubernetes/config"
	"github.com/fuseml/fuseml/cli/paas/config"
	"github.com/fuseml/fuseml/cli/paas/ui"
	"github.com/google/wire"
	"github.com/spf13/pflag"
)

// NewFusemlClient creates the Fuseml Client
func NewFusemlClient(flags *pflag.FlagSet, configOverrides func(*config.Config)) (*FusemlClient, func(), error) {
	wire.Build(
		wire.Struct(new(FusemlClient), "*"),
		config.Load,
		ui.NewUI,
		kubernetes.NewClusterFromClient,
		kubeconfig.KubeConfig,
		kubeconfig.NewClientLogger,
	)

	return &FusemlClient{}, func() {}, nil
}

// NewInstallClient creates the Fuseml Client for installation
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
