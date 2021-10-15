package deployments

import (
	"context"
	"fmt"
	"os"

	"github.com/fuseml/fuseml/cli/helpers"
	"github.com/fuseml/fuseml/cli/kubernetes"
	"github.com/fuseml/fuseml/cli/paas/ui"
	"github.com/kyokomi/emoji"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type Core struct {
	Debug           bool
	Timeout         int
	giteaCredSecret *corev1.Secret
}

const (
	// The deployment ID used for the fuseml-core service
	CoreDeploymentID        = "fuseml-core"
	coreDeploymentNamespace = "fuseml-core"
	coreIngressName         = "fuseml-core-ingress"
	coreServiceName         = "fuseml-core"
	coreServicePort         = 80
	coreSecretName          = "fuseml-core-gitea"
	coreConfigMapName       = "config-fuseml-core"
	coreDeploymentYamlPath  = "fuseml-core-deployment.yaml"
	coreVersion             = "0.1"
)

func (core *Core) ID() string {
	return CoreDeploymentID
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

	if err = c.Kubectl.CoreV1().ConfigMaps(coreDeploymentNamespace).Delete(context.Background(), coreConfigMapName, metav1.DeleteOptions{}); err != nil {
		return errors.Wrapf(err, "Failed deleting configMap %s", coreConfigMapName)
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

func createCoreIngress(c *kubernetes.Cluster, subdomain string) error {
	_, err := c.Kubectl.ExtensionsV1beta1().Ingresses(coreDeploymentNamespace).Create(
		context.Background(),
		&v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      coreIngressName,
				Namespace: coreDeploymentNamespace,
				Annotations: map[string]string{
					"kubernetes.io/ingress.class": "traefik",
				},
			},
			Spec: v1beta1.IngressSpec{
				Rules: []v1beta1.IngressRule{{
					Host: subdomain,
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{{
								Path: "/",
								Backend: v1beta1.IngressBackend{
									ServiceName: coreServiceName,
									ServicePort: intstr.IntOrString{
										Type:   intstr.Int,
										IntVal: 80,
									},
								}}}}}}}}},
		metav1.CreateOptions{},
	)
	return err
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
// however if GITEA_ADMIN_USERNAME, GITEA_ADMIN_PASSWORD, GITEA_URL variables are not set, internal gitea will be used
func (core Core) createCoreCredsSecret(c *kubernetes.Cluster) error {

	giteaUsername, exists := os.LookupEnv("GITEA_ADMIN_USERNAME")
	if !exists {
		if err := core.fetchGiteaCredSecret(c); err != nil {
			return errors.Wrap(err, "value for gitea admin user name (GITEA_ADMIN_USERNAME) was not provided neither found in installed gitea instance")
		}
		giteaUsername = string(core.giteaCredSecret.Data["username"])
	}

	giteaPassword, exists := os.LookupEnv("GITEA_ADMIN_PASSWORD")
	if !exists {
		if err := core.fetchGiteaCredSecret(c); err != nil {
			return errors.Wrap(err, "value for gitea admin user password (GITEA_ADMIN_PASSWORD) was not provided neither found in installed gitea instance")
		}
		giteaPassword = string(core.giteaCredSecret.Data["password"])
	}

	_, err := c.Kubectl.CoreV1().Secrets(coreDeploymentNamespace).Create(context.Background(),
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: coreSecretName,
			},
			StringData: map[string]string{
				"GITEA_ADMIN_USERNAME": giteaUsername,
				"GITEA_ADMIN_PASSWORD": giteaPassword,
			},
			Type: "Opaque",
		}, metav1.CreateOptions{})

	if err != nil {
		return err
	}
	return nil
}

func (core Core) createCoreConfigMap(c *kubernetes.Cluster, domain string) error {

	giteaURL, exists := os.LookupEnv("GITEA_URL")
	if !exists {
		giteaURL = "http://gitea." + domain
	}
	tektonURL, exists := os.LookupEnv("TEKTON_DASHBOARD_URL")
	if !exists {
		tektonURL = "http://tekton." + domain
	}

	_, err := c.Kubectl.CoreV1().ConfigMaps(coreDeploymentNamespace).Create(context.Background(),
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: coreConfigMapName,
			},
			Data: map[string]string{
				"GITEA_URL":            giteaURL,
				"TEKTON_DASHBOARD_URL": tektonURL,
			},
		}, metav1.CreateOptions{})

	if err != nil {
		return err
	}

	return nil
}

// Create fuseml-core deployment using the embedded template
func (core *Core) createCoreDeployment() error {

	yamlPathOnDisk, err := helpers.ExtractFile(coreDeploymentYamlPath)
	if err != nil {
		return errors.New("Failed to extract embedded file: " + coreDeploymentYamlPath + " - " + err.Error())
	}
	defer os.Remove(yamlPathOnDisk)

	out, err := helpers.Kubectl(fmt.Sprintf("apply --filename %s", yamlPathOnDisk))

	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("kubectl apply failed:\n%s", out))
	}
	return nil
}

// Install fuseml-core component
func (core Core) apply(c *kubernetes.Cluster, ui *ui.UI, options kubernetes.InstallationOptions, upgrade bool) error {
	if !upgrade {
		if err := core.createNamespace(c, ui); err != nil {
			return errors.Wrap(err, "Failed creating namespace for Core component")
		}
	}
	_, err := c.Kubectl.AppsV1().Deployments(coreDeploymentNamespace).Get(
		context.Background(),
		CoreDeploymentID,
		metav1.GetOptions{})

	if !upgrade && err == nil {

		ui.Exclamation().Msg(
			fmt.Sprintf("%s already present under %s namespace, skipping installation",
				CoreDeploymentID, coreDeploymentNamespace))
		return nil
	}

	domain, err := options.GetString("system_domain", CoreDeploymentID)
	if err != nil {
		return err
	}
	subdomain := CoreDeploymentID + "." + domain

	// delete existing secret and configMap to ensure we have the latest one after upgrade
	if upgrade {
		err = c.Kubectl.CoreV1().Secrets(coreDeploymentNamespace).Delete(context.Background(), coreSecretName, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "Failed deleting secret %s", coreSecretName)
		}
		err = c.Kubectl.CoreV1().ConfigMaps(coreDeploymentNamespace).Delete(context.Background(), coreConfigMapName, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "Failed deleting configMap %s", coreConfigMapName)
		}
	}

	if err := core.createCoreCredsSecret(c); err != nil {
		return errors.Wrap(err, "Failed creating secret for Core component")
	}
	if err := core.createCoreConfigMap(c, domain); err != nil {
		return errors.Wrap(err, "Failed creating configMap for Core component")
	}

	// create new deployment or upgrade existing one
	if err := core.createCoreDeployment(); err != nil {
		return errors.Wrap(err, fmt.Sprintf("Installing %s failed", coreDeploymentYamlPath))
	}

	if err := c.WaitUntilPodBySelectorExist(ui, coreDeploymentNamespace, "app.kubernetes.io/name="+CoreDeploymentID, core.Timeout); err != nil {
		return errors.Wrap(err, "failed waiting fuseml-core deployment to exist")
	}
	if err := c.WaitForPodBySelectorRunning(ui, coreDeploymentNamespace, "app.kubernetes.io/name="+CoreDeploymentID, core.Timeout); err != nil {
		return errors.Wrap(err, "failed waiting for fuseml-core deployment to come up")
	}

	if upgrade {
		ui.Success().Msg("FuseML core component successfully upgraded.")
		return nil
	}
	if c.HasIstio() {
		message := "Creating istio ingress gateway"
		out, err := helpers.WaitForCommandCompletion(ui, message,
			func() (string, error) {
				return helpers.CreateIstioIngressGateway(CoreDeploymentID, coreDeploymentNamespace, subdomain, coreServiceName, coreServicePort)
			},
		)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("%s failed:\n%s", message, out))
		}
	} else {
		message := "Creating ingress for fuseml-core"
		_, err = helpers.WaitForCommandCompletion(ui, message,
			func() (string, error) {
				return "", createCoreIngress(c, subdomain)
			},
		)
	}

	ui.Success().Msg(fmt.Sprintf("FuseML core component deployed (http://%s).", subdomain))

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

// Check if Core service is installed
func (core Core) Installed(c *kubernetes.Cluster) bool {

	_, err := c.Kubectl.CoreV1().Namespaces().Get(
		context.Background(),
		coreDeploymentNamespace,
		metav1.GetOptions{},
	)
	if apierrors.IsNotFound(err) {
		return false
	}
	_, err = c.Kubectl.AppsV1().Deployments(coreDeploymentNamespace).Get(
		context.Background(),
		CoreDeploymentID,
		metav1.GetOptions{})
	return err == nil
}

func (core Core) Upgrade(c *kubernetes.Cluster, ui *ui.UI, options kubernetes.InstallationOptions) error {
	if !core.Installed(c) {
		ui.Exclamation().Msg(
			fmt.Sprintf("%s not found in namespace %s. Upgrade not possible",
				CoreDeploymentID, coreDeploymentNamespace))
		return nil
	}
	return core.apply(c, ui, options, true)
}
