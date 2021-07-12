package deployments

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/fuseml/fuseml/cli/helpers"
	"github.com/fuseml/fuseml/cli/kubernetes"
	"github.com/fuseml/fuseml/cli/paas/ui"
	"github.com/kyokomi/emoji"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Registry struct {
	Debug   bool
	Timeout int
}

const (
	RegistryDeploymentID = "fuseml-registry"
	registryVersion      = "0.3.2"
	registryChartFile    = "trow-0.3.2.tgz"
)

func (k *Registry) ID() string {
	return RegistryDeploymentID
}

func (k *Registry) Backup(c *kubernetes.Cluster, ui *ui.UI, d string) error {
	return nil
}

func (k *Registry) Restore(c *kubernetes.Cluster, ui *ui.UI, d string) error {
	return nil
}

func (k Registry) Describe() string {
	return emoji.Sprintf(":cloud:Registry version: %s\n:clipboard:Registry chart: %s", registryVersion, registryChartFile)
}

// Delete removes Registry from kubernetes cluster
func (k Registry) Delete(c *kubernetes.Cluster, ui *ui.UI) error {
	ui.Note().KeeplineUnder(1).Msg("Removing Registry...")

	existsAndOwned, err := c.NamespaceExistsAndOwned(RegistryDeploymentID)
	if err != nil {
		return errors.Wrapf(err, "failed to check if namespace '%s' is owned or not", RegistryDeploymentID)
	}
	if !existsAndOwned {
		ui.Exclamation().Msg("Skipping Registry because namespace either doesn't exist or not owned by Fuseml")
		return nil
	}

	currentdir, err := os.Getwd()
	if err != nil {
		return errors.New("Failed uninstalling Registry: " + err.Error())
	}

	message := "Removing helm release " + RegistryDeploymentID
	out, err := helpers.WaitForCommandCompletion(ui, message,
		func() (string, error) {
			helmCmd := fmt.Sprintf("helm uninstall '%s' --namespace '%s'", RegistryDeploymentID, RegistryDeploymentID)
			return helpers.RunProc(helmCmd, currentdir, k.Debug)
		},
	)
	if err != nil {
		if strings.Contains(out, "release: not found") {
			ui.Exclamation().Msgf("%s helm release not found, skipping.\n", RegistryDeploymentID)
		} else {
			return errors.Wrapf(err, "Failed uninstalling helm release %s: %s", RegistryDeploymentID, out)
		}
	}

	message = "Deleting Registry namespace " + RegistryDeploymentID
	_, err = helpers.WaitForCommandCompletion(ui, message,
		func() (string, error) {
			return "", c.DeleteNamespace(RegistryDeploymentID)
		},
	)
	if err != nil {
		return errors.Wrapf(err, "Failed deleting namespace %s", RegistryDeploymentID)
	}

	ui.Success().Msg("Registry removed")

	return nil
}

func (k Registry) apply(c *kubernetes.Cluster, ui *ui.UI, options kubernetes.InstallationOptions, upgrade bool) error {
	action := "install"
	if upgrade {
		action = "upgrade"
	}

	currentdir, err := os.Getwd()
	if err != nil {
		return err
	}

	if action == "install" {
		helmCmd := fmt.Sprintf("helm list --namespace %s --deployed -q | grep %s", RegistryDeploymentID, RegistryDeploymentID)
		out, _ := helpers.RunProc(helmCmd, currentdir, k.Debug)
		if strings.TrimSpace(out) == RegistryDeploymentID {
			ui.Exclamation().Msg(RegistryDeploymentID + " already present under " + RegistryDeploymentID + " namespace, skipping installation")
			return nil
		}
	}

	tarPath, err := helpers.ExtractFile(registryChartFile)
	if err != nil {
		return errors.New("Failed to extract embedded file: " + tarPath + " - " + err.Error())
	}
	defer os.Remove(tarPath)

	config := fmt.Sprintf(`
trow:
  domain: registry.%s

fullnameOverride: registry
`, RegistryDeploymentID)

	configPath, err := helpers.CreateTmpFile(config)
	if err != nil {
		return err
	}
	defer os.Remove(configPath)

	helmCmd := fmt.Sprintf("helm %s %s --values %s --create-namespace --namespace %s %s", action, RegistryDeploymentID, configPath, RegistryDeploymentID, tarPath)
	if out, err := helpers.RunProc(helmCmd, currentdir, k.Debug); err != nil {
		return errors.New("Failed installing Registry: " + out)
	}

	err = c.LabelNamespace(RegistryDeploymentID, kubernetes.FusemlDeploymentLabelKey, kubernetes.FusemlDeploymentLabelValue)
	if err != nil {
		return err
	}
	if err := c.WaitUntilPodBySelectorExist(ui, RegistryDeploymentID, "app.kubernetes.io/name=trow", 180); err != nil {
		return errors.Wrap(err, "failed waiting Registry deployment to come up")
	}
	if err := c.WaitForPodBySelectorRunning(ui, RegistryDeploymentID, "app.kubernetes.io/name=trow", 180); err != nil {
		return errors.Wrap(err, "failed waiting Registry deployment to come up")
	}

	ui.Success().Msg("Registry deployed")

	return nil
}

func (k Registry) GetVersion() string {
	return registryVersion
}

func (k Registry) Deploy(c *kubernetes.Cluster, ui *ui.UI, options kubernetes.InstallationOptions) error {

	_, err := c.Kubectl.CoreV1().Namespaces().Get(
		context.Background(),
		RegistryDeploymentID,
		metav1.GetOptions{},
	)
	if err == nil {
		ui.Note().Msg("Namespace " + RegistryDeploymentID + " already present")
	}

	ui.Note().KeeplineUnder(1).Msg("Deploying Registry...")

	err = k.apply(c, ui, options, false)
	if err != nil {
		return err
	}

	return nil
}

func (k Registry) Upgrade(c *kubernetes.Cluster, ui *ui.UI, options kubernetes.InstallationOptions) error {
	_, err := c.Kubectl.CoreV1().Namespaces().Get(
		context.Background(),
		RegistryDeploymentID,
		metav1.GetOptions{},
	)
	if err != nil {
		return errors.New("Namespace " + RegistryDeploymentID + " not present")
	}

	ui.Note().Msg("Upgrading Registry...")

	return k.apply(c, ui, options, true)
}
