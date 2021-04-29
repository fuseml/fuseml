package deployments

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"

	"github.com/fuseml/fuseml/cli/helpers"
	"github.com/fuseml/fuseml/cli/kubernetes"
	"github.com/fuseml/fuseml/cli/paas/ui"
	"github.com/kyokomi/emoji"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Core struct {
	Debug           bool
	Timeout         int
	giteaCredSecret *corev1.Secret
}

const (
	coreDeploymentID        = "fuseml-core"
	coreDeploymentNamespace = "fuseml-core"
	coreServiceName         = "fuseml-core"
	coreServicePort         = 80
	coreSecretName          = "fuseml-core-gitea"
	coreDeploymentYamlPath  = "fuseml-core-deployment.yaml"
	coreVersion             = "0.1"
)

func (core *Core) ID() string {
	return coreDeploymentID
}

func (core *Core) Backup(c *kubernetes.Cluster, ui *ui.UI, d string) error {
	return nil
}

func (core *Core) Restore(c *kubernetes.Cluster, ui *ui.UI, d string) error {
	return nil
}

func (core Core) Describe() string {
	return emoji.Sprintf(":cloud:Core version: %s", coreVersion)
}

// Delete removes Core component from kubernetes cluster
func (core Core) Delete(c *kubernetes.Cluster, ui *ui.UI) error {
	ui.Note().KeeplineUnder(1).Msg("Removing Core component...")

	existsAndOwned, err := c.NamespaceExistsAndOwned(coreDeploymentNamespace)
	if err != nil {
		return errors.Wrapf(err, "failed to check if namespace '%s' is owned or not", coreDeploymentNamespace)
	}
	if !existsAndOwned {
		ui.Exclamation().Msg("Skipping Core component removal because namespace either doesn't exist or not owned by FuseML")
		return nil
	}

	if out, err := helpers.KubectlDeleteEmbeddedYaml(coreDeploymentYamlPath, true); err != nil {
		return errors.Wrap(err, fmt.Sprintf("Deleting %s failed:\n%s", coreDeploymentYamlPath, out))
	}
	if err = c.Kubectl.CoreV1().Secrets(coreDeploymentNamespace).Delete(context.Background(), coreSecretName, metav1.DeleteOptions{}); err != nil {
		return errors.Wrapf(err, "Failed deleting secret %s", coreSecretName)
	}

	message := "Deleting Core component namespace " + coreDeploymentNamespace
	_, err = helpers.WaitForCommandCompletion(ui, message,
		func() (string, error) {
			return "", c.DeleteNamespace(coreDeploymentNamespace)
		},
	)
	if err != nil {
		return errors.Wrapf(err, "Failed deleting namespace %s", coreDeploymentNamespace)
	}

	message = "Waiting for Core namespace to be gone"
	warning, err := helpers.WaitForCommandCompletion(ui, message,
		func() (string, error) {
			var err error
			for err == nil {
				_, err = c.Kubectl.CoreV1().Namespaces().Get(
					context.Background(),
					coreDeploymentNamespace,
					metav1.GetOptions{},
				)
			}
			if serr, ok := err.(*apierrors.StatusError); ok {
				if serr.ErrStatus.Reason == metav1.StatusReasonNotFound {
					return "", nil
				}
			}

			return "", err
		},
	)
	if err != nil {
		return err
	}
	if warning != "" {
		ui.Exclamation().Msg(warning)
	}

	ui.Success().Msg("Core component removed")

	return nil
}

// Create kubernetes namespace for core component
func (core Core) createNamespace(c *kubernetes.Cluster, ui *ui.UI) error {
	if _, err := c.Kubectl.CoreV1().Namespaces().Create(
		context.Background(),
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: coreDeploymentNamespace,
				Labels: map[string]string{
					kubernetes.FusemlDeploymentLabelKey: kubernetes.FusemlDeploymentLabelValue,
				},
			},
		},
		metav1.CreateOptions{},
	); err != nil {
		return nil
	}

	if err := c.LabelNamespace(coreDeploymentNamespace, kubernetes.FusemlDeploymentLabelKey, kubernetes.FusemlDeploymentLabelValue); err != nil {
		return err
	}

	return nil
}

func (core *Core) fetchGiteaCredSecret(c *kubernetes.Cluster) error {
	if core.giteaCredSecret != nil {
		return nil
	}
	giteaCredSecret, err := c.Kubectl.CoreV1().Secrets("fuseml-workloads").
		Get(context.Background(), "gitea-creds", metav1.GetOptions{})
	if err != nil {
		return err
	}
	core.giteaCredSecret = giteaCredSecret
	return nil
}

// Create secret to store gitea credentials required by fuseml-core deployment
// It is possible for user to provide access to different gitea instance than the one deployed by fuseml-installer,
// however if GITEA_USERNAME, GITEA_PASSWORD, GITEA_URL variables are not set, internal gitea will be used
func (core Core) createCoreCredsSecret(c *kubernetes.Cluster) error {

	giteaUsername, exists := os.LookupEnv("GITEA_USERNAME")
	if !exists {
		if err := core.fetchGiteaCredSecret(c); err != nil {
			return errors.Wrap(err, "value for gitea user name (GITEA_USERNAME) was not provided neither found in installed gitea instance")
		}
		giteaUsername = string(core.giteaCredSecret.Data["username"])
	}

	giteaPassword, exists := os.LookupEnv("GITEA_PASSWORD")
	if !exists {
		if err := core.fetchGiteaCredSecret(c); err != nil {
			return errors.Wrap(err, "value for gitea user password (GITEA_PASSWORD) was not provided neither found in installed gitea instance")
		}
		giteaPassword = string(core.giteaCredSecret.Data["password"])
	}

	_, err := c.Kubectl.CoreV1().Secrets(coreDeploymentNamespace).Create(context.Background(),
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: coreSecretName,
			},
			StringData: map[string]string{
				"GITEA_USERNAME": giteaUsername,
				"GITEA_PASSWORD": giteaPassword,
			},
			Type: "Opaque",
		}, metav1.CreateOptions{})

	if err != nil {
		return err
	}
	return nil
}

// Create fuseml-core deployment pointing to provided gitea server URL
func (core *Core) createCoreDeployment(giteaURL string) error {

	yamlPathOnDisk, err := helpers.ExtractFile(coreDeploymentYamlPath)
	if err != nil {
		return errors.New("Failed to extract embedded file: " + coreDeploymentYamlPath + " - " + err.Error())
	}
	defer os.Remove(yamlPathOnDisk)

	fileContents, err := ioutil.ReadFile(yamlPathOnDisk)
	if err != nil {
		return err
	}

	re := regexp.MustCompile(`__GITEA_URL__`)
	renderedFileContents := re.ReplaceAllString(string(fileContents), giteaURL)

	tmpFilePath, err := helpers.CreateTmpFile(string(renderedFileContents))
	if err != nil {
		return err
	}
	defer os.Remove(tmpFilePath)

	out, err := helpers.Kubectl(fmt.Sprintf("apply -n %s --filename %s", coreDeploymentNamespace, tmpFilePath))

	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("kubectl apply failed:\n%s", out))
	}
	return nil
}

// Install fuseml-core component
func (core Core) apply(c *kubernetes.Cluster, ui *ui.UI, options kubernetes.InstallationOptions, upgrade bool) error {
	if upgrade {
		ui.Note().Msg("Upgrade operation not implemented...")
		return nil
	}

	if err := core.createNamespace(c, ui); err != nil {
		return err
	}
	// FIXME check for possible deployment presence here

	if err := core.createCoreCredsSecret(c); err != nil {
		return err
	}

	domain, err := options.GetString("system_domain", coreDeploymentID)
	if err != nil {
		return err
	}
	subdomain := coreDeploymentID + "." + domain

	giteaURL, exists := os.LookupEnv("GITEA_URL")
	if !exists {
		giteaURL = "http://gitea." + domain
	}
	if err := core.createCoreDeployment(giteaURL); err != nil {
		return errors.Wrap(err, fmt.Sprintf("Installing %s failed", coreDeploymentYamlPath))
	}

	if err := c.WaitUntilPodBySelectorExist(ui, coreDeploymentNamespace, "app.kubernetes.io/name="+coreDeploymentID, core.Timeout); err != nil {
		return errors.Wrap(err, "failed waiting fuseml-core deployment to exist")
	}
	if err := c.WaitForPodBySelectorRunning(ui, coreDeploymentNamespace, "app.kubernetes.io/name="+coreDeploymentID, core.Timeout); err != nil {
		return errors.Wrap(err, "failed waiting for fuseml-core deployment to come up")
	}

	if c.HasIstio() {
		message := "Creating istio ingress gateway"
		out, err := helpers.WaitForCommandCompletion(ui, message,
			func() (string, error) {
				return helpers.CreateIstioIngressGateway(coreDeploymentID, coreDeploymentNamespace, subdomain, coreServiceName, coreServicePort)
			},
		)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("%s failed:\n%s", message, out))
		}
	}
	// TODO else install ingress

	ui.Success().Msg("Core component deployed")

	return nil
}

func (core Core) GetVersion() string {
	return coreVersion
}

func (core Core) Deploy(c *kubernetes.Cluster, ui *ui.UI, options kubernetes.InstallationOptions) error {

	_, err := c.Kubectl.CoreV1().Namespaces().Get(
		context.Background(),
		coreDeploymentNamespace,
		metav1.GetOptions{},
	)
	if err == nil {
		ui.Note().Msg("Namespace " + coreDeploymentNamespace + " already present")
	}

	ui.Note().KeeplineUnder(1).Msg("Deploying Core...")

	err = core.apply(c, ui, options, false)
	if err != nil {
		return err
	}

	return nil
}

func (core Core) Upgrade(c *kubernetes.Cluster, ui *ui.UI, options kubernetes.InstallationOptions) error {
	ui.Note().Msg("Upgrade operation not implemented...")
	return nil
}
