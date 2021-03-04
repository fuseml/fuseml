package deployments

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/kyokomi/emoji"
	"github.com/pkg/errors"
	"github.com/suse/carrier/cli/helpers"
	"github.com/suse/carrier/cli/kubernetes"
	"github.com/suse/carrier/cli/paas/ui"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MLflow struct {
	Debug   bool
	Timeout int
}

const (
	MLflowDeploymentID = "mlflow"
	mlflowNamespace    = "carrier-workloads"
	mlflowVersion      = "0.0.1"
	mlflowChartFile    = "mlflow-0.0.1.tgz"
)

func (k *MLflow) ID() string {
	return MLflowDeploymentID
}

func (k *MLflow) Backup(c *kubernetes.Cluster, ui *ui.UI, d string) error {
	return nil
}

func (k *MLflow) Restore(c *kubernetes.Cluster, ui *ui.UI, d string) error {
	return nil
}

func (k MLflow) Describe() string {
	return emoji.Sprintf(":cloud:MLflow version: %s\n:clipboard:MLflow chart: %s", mlflowVersion, mlflowChartFile)
}

// Delete removes MLflow from kubernetes cluster
func (k MLflow) Delete(c *kubernetes.Cluster, ui *ui.UI) error {
	ui.Note().KeeplineUnder(1).Msg("Removing MLflow...")

	existsAndOwned, err := c.NamespaceExistsAndOwned(mlflowNamespace)
	if err != nil {
		return errors.Wrapf(err, "failed to check if namespace '%s' is owned or not", mlflowNamespace)
	}
	if !existsAndOwned {
		ui.Exclamation().Msg("Skipping MLflow because namespace either doesn't exist or not owned by Carrier")
		return nil
	}

	currentdir, err := os.Getwd()
	if err != nil {
		return errors.New("Failed uninstalling MLflow: " + err.Error())
	}

	message := "Removing helm release " + MLflowDeploymentID
	out, err := helpers.WaitForCommandCompletion(ui, message,
		func() (string, error) {
			helmCmd := fmt.Sprintf("helm uninstall '%s' --namespace '%s'", MLflowDeploymentID, mlflowNamespace)
			return helpers.RunProc(helmCmd, currentdir, k.Debug)
		},
	)
	if err != nil {
		if strings.Contains(out, "release: not found") {
			ui.Exclamation().Msgf("%s helm release not found, skipping.\n", MLflowDeploymentID)
		} else {
			return errors.Wrapf(err, "Failed uninstalling helm release %s: %s", MLflowDeploymentID, out)
		}
	}

	message = "Deleting MLflow namespace " + MLflowDeploymentID
	_, err = helpers.WaitForCommandCompletion(ui, message,
		func() (string, error) {
			return "", c.DeleteNamespace(mlflowNamespace)
		},
	)
	if err != nil {
		return errors.Wrapf(err, "Failed deleting namespace %s", mlflowNamespace)
	}

	ui.Success().Msg("MLflow removed")

	return nil
}

func (k MLflow) apply(c *kubernetes.Cluster, ui *ui.UI, options kubernetes.InstallationOptions, upgrade bool) error {
	action := "install"
	if upgrade {
		action = "upgrade"
	}

	currentdir, err := os.Getwd()
	if err != nil {
		return err
	}

	tarPath, err := helpers.ExtractFile(mlflowChartFile)
	if err != nil {
		return errors.New("Failed to extract embedded file: " + tarPath + " - " + err.Error())
	}
	defer os.Remove(tarPath)

	domain, err := options.GetString("system_domain", MLflowDeploymentID)
	if err != nil {
		return err
	}
	subdomain := MLflowDeploymentID + "." + domain

	config := fmt.Sprintf(`
expose:
  tls:
    enabled: false
  ingress:
    hosts:
      - %s
    annotations:
      kubernetes.io/ingress.class: traefik

minio:
  ingress:
    hosts:
      - %s
    annotations:
      kubernetes.io/ingress.class: traefik
`, subdomain, "minio."+subdomain)

	configPath, err := helpers.CreateTmpFile(config)
	if err != nil {
		return err
	}
	defer os.Remove(configPath)

	helmCmd := fmt.Sprintf("helm list --namespace %s -q | grep %s", mlflowNamespace, MLflowDeploymentID)
	out, err := helpers.RunProc(helmCmd, currentdir, k.Debug)
	if strings.TrimSpace(out) == MLflowDeploymentID {
		ui.Exclamation().Msg(MLflowDeploymentID + " already present under " + mlflowNamespace + " namespace, skipping installation")
		return nil
	}

	helmCmd = fmt.Sprintf("helm %s %s --create-namespace --values %s --namespace %s %s", action, MLflowDeploymentID, configPath, mlflowNamespace, tarPath)
	if out, err = helpers.RunProc(helmCmd, currentdir, k.Debug); err != nil {
		return errors.New("Failed installing MLflow: " + out)
	}

	err = c.LabelNamespace(mlflowNamespace, kubernetes.CarrierDeploymentLabelKey, kubernetes.CarrierDeploymentLabelValue)
	if err != nil {
		return err
	}

	for _, podname := range []string{
		"minio",
		"mysql",
		"mlflow",
	} {
		if err := c.WaitUntilPodBySelectorExist(ui, mlflowNamespace, "app.kubernetes.io/name="+podname, k.Timeout); err != nil {
			return errors.Wrap(err, "failed waiting MLflow "+podname+" deployment to exist")
		}
		if err := c.WaitForPodBySelectorRunning(ui, mlflowNamespace, "app.kubernetes.io/name="+podname, k.Timeout); err != nil {
			return errors.Wrap(err, "failed waiting MLflow "+podname+" deployment to come up")
		}
	}

	ui.Success().Msg("MLflow deployed")

	return nil
}

func (k MLflow) GetVersion() string {
	return mlflowVersion
}

func (k MLflow) Deploy(c *kubernetes.Cluster, ui *ui.UI, options kubernetes.InstallationOptions) error {

	// FIXME do not check for namespace presence, it is not installed into its own
	/*
		_, err := c.Kubectl.CoreV1().Namespaces().Get(
			context.Background(),
			MLflowDeploymentID,
			metav1.GetOptions{},
		)
		if err == nil {
			ui.Exclamation().Msg("Namespace " + mlflowNamespace + " already present, skipping installation")
			return nil
		}
	*/

	ui.Note().KeeplineUnder(1).Msg("Deploying MLflow...")

	err := k.apply(c, ui, options, false)
	if err != nil {
		return err
	}

	return nil
}

func (k MLflow) Upgrade(c *kubernetes.Cluster, ui *ui.UI, options kubernetes.InstallationOptions) error {
	_, err := c.Kubectl.CoreV1().Namespaces().Get(
		context.Background(),
		MLflowDeploymentID,
		metav1.GetOptions{},
	)
	if err != nil {
		return errors.New("Namespace " + mlflowNamespace + " not present")
	}

	ui.Note().Msg("Upgrading MLflow...")

	return k.apply(c, ui, options, true)
}
