package paas

import (
	"github.com/fuseml/fuseml/cli/kubernetes"
	"github.com/fuseml/fuseml/cli/paas/config"
	"github.com/fuseml/fuseml/cli/paas/ui"
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
