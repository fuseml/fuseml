package paas

import (
	"github.com/fuseml/fuseml/cli/kubernetes"
	"github.com/fuseml/fuseml/cli/paas/config"
	"github.com/fuseml/fuseml/cli/paas/ui"
	"github.com/fuseml/fuseml/cli/paas/version"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
)

// FusemlClient provides functionality for talking to a
// Fuseml installation on Kubernetes
type FusemlClient struct {
	kubeClient *kubernetes.Cluster
	ui         *ui.UI
	config     *config.Config
	Log        logr.Logger
}

// Info displays information about environment
func (c *FusemlClient) Info() error {
	log := c.Log.WithName("Info")
	log.Info("start")
	defer log.Info("return")

	platform := c.kubeClient.GetPlatform()
	kubeVersion, err := c.kubeClient.GetVersion()
	if err != nil {
		return errors.Wrap(err, "failed to get kube version")
	}

	c.ui.Success().
		WithStringValue("Platform", platform.String()).
		WithStringValue("Kubernetes Version", kubeVersion).
		Msg("Fuseml Environment")

	return nil
}

// Version displays version information about the installer
func (c *FusemlClient) Version() error {
	log := c.Log.WithName("version")
	log.Info("start")
	defer log.Info("return")

	version := version.GetInfo()

	c.ui.Success().
		WithStringValue("Version", version.Version).
		WithStringValue("GitCommit", version.GitCommit).
		WithStringValue("Build Date", version.BuildDate).
		WithStringValue("Go Version", version.GoVersion).
		WithStringValue("Compiler", version.Compiler).
		WithStringValue("Platform", version.Platform).
		Msg("Fuseml Installer")

	return nil
}
