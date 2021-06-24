package deployments

import (
	"fmt"

	"github.com/fuseml/fuseml/cli/kubernetes"
	"github.com/fuseml/fuseml/cli/paas/ui"
)

type Extension struct {
	Name        string
	Repository  string
	Description string
	Namespace   string
}

func NewExtension(name, repository string) *Extension {
	return &Extension{
		Name:       name,
		Repository: repository,
	}
}

func (e Extension) LoadDescription() error {

	// 1. check presence and format of Repository
	// 2. check the presence of description file
	// 3. parse and validate descriptin file into Extension struct
	return nil
}

func (e Extension) Install(c *kubernetes.Cluster, ui *ui.UI, options *kubernetes.InstallationOptions) error {

	// TODO based on installation type (script/helm/manifest), proceed with installation

	// TODO installation steps could have different namespaces...

	err := c.LabelNamespace(e.Namespace, kubernetes.FusemlDeploymentLabelKey, kubernetes.FusemlDeploymentLabelValue)
	if err != nil {
		return err
	}

	// TODO wait for some pod to exist/run? Extra option in the description file

	// TODO after installation, we might need to create istio ingress gateway! That means extra kubernetes manifest
	// (or maybe just boolean value indicating right functions are written from here)

	ui.Success().Msg(fmt.Sprintf("%s deployed.", e.Name))

	return nil
}
