package deployments

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/fuseml/fuseml/cli/helpers"
	"github.com/fuseml/fuseml/cli/kubernetes"
	"github.com/fuseml/fuseml/cli/paas/ui"
	"github.com/kyokomi/emoji"
	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Istio struct {
	Debug   bool
	Timeout int
}

const (
	istioDeploymentID        = "istio"
	istioDeploymentNamespace = "istio-system"
	istioVersion             = "1.11.4"
	istioFetchScriptURL      = "https://git.io/getLatestIstio"
	istioFetchScript         = "getLatestIstio.sh"
	istioOperatorYamlPath    = "istio/istio-minimal-operator.yaml"
)

func (i *Istio) ID() string {
	return istioDeploymentID
}

func (i *Istio) Backup(c *kubernetes.Cluster, ui *ui.UI, d string) error {
	return nil
}

func (i *Istio) Restore(c *kubernetes.Cluster, ui *ui.UI, d string) error {
	return nil
}

func (i Istio) Describe() string {
	return emoji.Sprintf(":cloud:Istio version: %s", istioVersion)
}

// Delete removes istio from kubernetes cluster
func (i Istio) Delete(c *kubernetes.Cluster, ui *ui.UI) error {
	ui.Note().KeeplineUnder(1).Msg("Removing Istio...")

	existsAndOwned, err := c.NamespaceExistsAndOwned(istioDeploymentNamespace)
	if err != nil {
		return errors.Wrapf(err, "failed to check if namespace '%s' is owned or not", istioDeploymentNamespace)
	}
	if !existsAndOwned {
		ui.Exclamation().Msg("Skipping istio component removal because namespace either doesn't exist or not owned by FuseML")
		return nil
	}

	tmpDir, err := ioutil.TempDir("", "istio-uninstall")
	if err != nil {
		return errors.Wrap(err, "can't create temp directory")
	}
	defer os.Remove(tmpDir)

	istioctl, err := i.fetchIstioctl(tmpDir)
	if err != nil {
		return errors.Wrap(err, "can't download istioctl")
	}

	yamlPathOnDisk, err := helpers.ExtractFile(istioOperatorYamlPath)
	if err != nil {
		return errors.New("Failed to extract embedded file: " + istioOperatorYamlPath + " - " + err.Error())
	}
	defer os.Remove(yamlPathOnDisk)

	fullCmd := istioctl + " manifest generate -f " + yamlPathOnDisk + "| kubectl delete --ignore-not-found -f -"
	if out, err := helpers.RunProc(fullCmd, tmpDir, i.Debug); err != nil {
		return errors.Wrap(err, fmt.Sprintf("Failed uninstalling istio: %s\n", out))
	}

	message := "Deleting Istio namespace " + istioDeploymentNamespace
	_, err = helpers.WaitForCommandCompletion(ui, message,
		func() (string, error) {
			return "", c.DeleteNamespace(istioDeploymentNamespace)
		},
	)
	if err != nil {
		return errors.Wrapf(err, "Failed deleting namespace %s", istioDeploymentNamespace)
	}

	return nil
}

// Create kubernetes namespace for istio
func (i Istio) createNamespace(c *kubernetes.Cluster, ui *ui.UI) error {
	_, err := c.Kubectl.CoreV1().Namespaces().Create(
		context.Background(),
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: istioDeploymentNamespace,
				Labels: map[string]string{
					"istio-injection": "disabled",
				},
			},
		},
		metav1.CreateOptions{},
	)
	if err != nil {
		return nil
	}

	if err := c.LabelNamespace(istioDeploymentNamespace, kubernetes.FusemlDeploymentLabelKey, kubernetes.FusemlDeploymentLabelValue); err != nil {
		return err
	}

	return nil
}

func (i Istio) apply(c *kubernetes.Cluster, ui *ui.UI, options kubernetes.InstallationOptions, upgrade bool) error {
	if upgrade {
		return nil
	}

	if err := i.createNamespace(c, ui); err != nil {
		return errors.Wrap(err, "Failed creating namespace for istio component")
	}

	tmpDir, err := ioutil.TempDir("", "istio-install")
	if err != nil {
		return errors.Wrap(err, "can't create temp directory")
	}
	defer os.Remove(tmpDir)

	istioctl, err := i.fetchIstioctl(tmpDir)
	if err != nil {
		return errors.Wrap(err, "can't download istioctl")
	}

	yamlPathOnDisk, err := helpers.ExtractFile(istioOperatorYamlPath)
	if err != nil {
		return errors.New("Failed to extract embedded file: " + istioOperatorYamlPath + " - " + err.Error())
	}
	defer os.Remove(yamlPathOnDisk)

	fullCmd := istioctl + " manifest install -yf " + yamlPathOnDisk
	if out, err := helpers.RunProc(fullCmd, tmpDir, i.Debug); err != nil {
		return errors.Wrap(err, fmt.Sprintf("Failed installing istio: %s\n", out))
	}

	if err := c.WaitUntilPodBySelectorExist(ui, istioDeploymentNamespace, "app=istiod", i.Timeout); err != nil {
		return errors.Wrap(err, "failed waiting for Istio deployment to exist")
	}
	if err := c.WaitForPodBySelectorRunning(ui, istioDeploymentNamespace, "app=istiod", i.Timeout); err != nil {
		return errors.Wrap(err, "failed waiting for Istio deployment to come up")
	}

	ui.Success().Msg("Istio deployed")

	return nil
}

func (i Istio) GetVersion() string {
	return istioVersion
}

func (i Istio) Deploy(c *kubernetes.Cluster, ui *ui.UI, options kubernetes.InstallationOptions) error {

	if c.HasIstio() {
		ui.Exclamation().Msg("Istio already installed, skipping ...")
		return nil
	}

	_, err := c.Kubectl.CoreV1().Services("kube-system").Get(
		context.Background(),
		"traefik",
		metav1.GetOptions{},
	)
	if err == nil {
		ui.Exclamation().Msg("Traefik Ingress already installed, not installing Istio")
		return nil
	}

	ui.Note().KeeplineUnder(1).Msg("Deploying Istio...")

	return i.apply(c, ui, options, false)
}

func (k Istio) Upgrade(c *kubernetes.Cluster, ui *ui.UI, options kubernetes.InstallationOptions) error {
	ui.Note().Msg("Upgrade not supported")
	return nil
}

func (i Istio) fetchIstioctl(dir string) (string, error) {
	err := helpers.DownloadFile(istioFetchScriptURL, istioFetchScript, dir)
	if err != nil {
		return "", errors.New(fmt.Sprintf("Failed downloading install script from %s: %s", istioFetchScriptURL, err.Error()))
	}

	pathToFetchScript := filepath.Join(dir, istioFetchScript)

	if err := os.Chmod(pathToFetchScript, 0750); err != nil {
		return "", errors.New(fmt.Sprintf("Failed changing the file mode of %s", pathToFetchScript))
	}

	env := []string{"ISTIO_VERSION=" + i.GetVersion()}
	if out, err := helpers.RunProcEnv(pathToFetchScript, dir, i.Debug, env); err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("Failed downloading istio: %s\n", out))
	}
	return filepath.Join(dir, "istio-"+i.GetVersion(), "bin", "istioctl"), nil
}
