package deployments

import (
	"context"
	"fmt"
	"strconv"

	"github.com/fuseml/fuseml/cli/helpers"
	"github.com/fuseml/fuseml/cli/kubernetes"
	"github.com/fuseml/fuseml/cli/paas/ui"
	"github.com/kyokomi/emoji"
	"github.com/pkg/errors"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type Tekton struct {
	Debug      bool
	Secrets    []string
	ConfigMaps []string
	Timeout    int
}

const (
	TektonDeploymentID        = "tekton"
	tektonNamespace           = "tekton-pipelines"
	tektonPipelineYamlPath    = "tekton/install/pipeline-v0.22.0.yaml"
	tektonTriggersYamlPath    = "tekton/install/triggers-v0.12.1.yaml"
	tektonDashboardYamlPath   = "tekton/install/dashboard-v0.15.0.yaml"
	tektonAdminRoleYamlPath   = "tekton/install/admin-role.yaml"
	tektonFuseMLTasksYamlPath = "tekton/tasks"
)

var fuseMLTasks = []string{"clone", "kaniko", "builder-prep"}

func (k *Tekton) ID() string {
	return TektonDeploymentID
}

func (k *Tekton) Backup(c *kubernetes.Cluster, ui *ui.UI, d string) error {
	return nil
}

func (k *Tekton) Restore(c *kubernetes.Cluster, ui *ui.UI, d string) error {
	return nil
}

func (k Tekton) Describe() string {
	return emoji.Sprintf(":cloud:Tekton pipeline: %s\n:cloud:Tekton dashboard: %s\n:cloud:Tekton triggers: %s\n",
		tektonPipelineYamlPath, tektonDashboardYamlPath, tektonTriggersYamlPath)
}

// Delete removes Tekton from kubernetes cluster
func (k Tekton) Delete(c *kubernetes.Cluster, ui *ui.UI) error {
	ui.Note().KeeplineUnder(1).Msg("Removing Tekton...")

	existsAndOwned, err := c.NamespaceExistsAndOwned(tektonNamespace)
	if err != nil {
		return errors.Wrapf(err, "failed to check if namespace '%s' is owned or not", tektonNamespace)
	}
	if !existsAndOwned {
		ui.Exclamation().Msg("Skipping Tekton because namespace either doesn't exist or not owned by Fuseml")
		return nil
	}

	if out, err := helpers.KubectlDeleteEmbeddedYaml(tektonDashboardYamlPath, true); err != nil {
		return errors.Wrap(err, fmt.Sprintf("Deleting %s failed:\n%s", tektonDashboardYamlPath, out))
	}
	if out, err := helpers.KubectlDeleteEmbeddedYaml(tektonAdminRoleYamlPath, true); err != nil {
		return errors.Wrap(err, fmt.Sprintf("Deleting %s failed:\n%s", tektonAdminRoleYamlPath, out))
	}
	if out, err := helpers.KubectlDeleteEmbeddedYaml(tektonTriggersYamlPath, true); err != nil {
		return errors.Wrap(err, fmt.Sprintf("Deleting %s failed:\n%s", tektonTriggersYamlPath, out))
	}
	if out, err := helpers.KubectlDeleteEmbeddedYaml(tektonPipelineYamlPath, true); err != nil {
		return errors.Wrap(err, fmt.Sprintf("Deleting %s failed:\n%s", tektonPipelineYamlPath, out))
	}

	message := "Deleting Tekton namespace " + tektonNamespace
	_, err = helpers.WaitForCommandCompletion(ui, message,
		func() (string, error) {
			return "", c.DeleteNamespace(tektonNamespace)
		},
	)
	if err != nil {
		return errors.Wrapf(err, "Failed deleting namespace %s", tektonNamespace)
	}

	ui.Success().Msg("Tekton removed")

	return nil
}

func (k Tekton) apply(c *kubernetes.Cluster, ui *ui.UI, options kubernetes.InstallationOptions, upgrade bool) error {
	if out, err := helpers.KubectlApplyEmbeddedYaml(tektonAdminRoleYamlPath); err != nil {
		return errors.Wrap(err, fmt.Sprintf("Installing %s failed:\n%s", tektonAdminRoleYamlPath, out))
	}
	if out, err := helpers.KubectlApplyEmbeddedYaml(tektonPipelineYamlPath); err != nil {
		return errors.Wrap(err, fmt.Sprintf("Installing %s failed:\n%s", tektonPipelineYamlPath, out))
	}
	if out, err := helpers.KubectlApplyEmbeddedYaml(tektonTriggersYamlPath); err != nil {
		return errors.Wrap(err, fmt.Sprintf("Installing %s failed:\n%s", tektonTriggersYamlPath, out))
	}
	if out, err := helpers.KubectlApplyEmbeddedYaml(tektonDashboardYamlPath); err != nil {
		return errors.Wrap(err, fmt.Sprintf("Installing %s failed:\n%s", tektonDashboardYamlPath, out))
	}

	err := c.LabelNamespace(tektonNamespace, kubernetes.FusemlDeploymentLabelKey, kubernetes.FusemlDeploymentLabelValue)
	if err != nil {
		return err
	}

	for _, crd := range []string{
		"clustertasks.tekton.dev",
		"clustertriggerbindings.triggers.tekton.dev",
		"conditions.tekton.dev",
		"eventlisteners.triggers.tekton.dev",
		"pipelineresources.tekton.dev",
		"pipelineruns.tekton.dev",
		"pipelines.tekton.dev",
		"runs.tekton.dev",
		"taskruns.tekton.dev",
		"tasks.tekton.dev",
		"triggerbindings.triggers.tekton.dev",
		"triggers.triggers.tekton.dev",
		"triggertemplates.triggers.tekton.dev",
	} {
		message := fmt.Sprintf("Establish CRD %s", crd)
		out, err := helpers.WaitForCommandCompletion(ui, message,
			func() (string, error) {
				return helpers.Kubectl("wait --for=condition=established --timeout=" + strconv.Itoa(k.Timeout) + "s crd/" + crd)
			},
		)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("%s failed:\n%s", message, out))
		}
	}

	for _, c := range []string{"pipelines", "triggers", "dashboard"} {
		message := fmt.Sprintf("Starting tekton %s pods", c)
		out, err := helpers.WaitForCommandCompletion(ui, message,
			func() (string, error) {
				return helpers.Kubectl(fmt.Sprintf("wait --for=condition=Ready --timeout=%ds -n %s --selector=app.kubernetes.io/part-of=tekton-%s pod",
					k.Timeout, tektonNamespace, c))
			},
		)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("%s failed:\n%s", message, out))
		}
	}

	for _, task := range fuseMLTasks {
		message := fmt.Sprintf("Installing FuseML task: %s", task)
		out, err := helpers.WaitForCommandCompletion(ui, message,
			func() (string, error) {
				return helpers.KubectlApplyEmbeddedYaml(fmt.Sprintf("%s/%s.yaml", tektonFuseMLTasksYamlPath, task))
			},
		)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("%s failed:\n%s", message, out))
		}

	}

	domain, err := options.GetString("system_domain", TektonDeploymentID)
	if err != nil {
		return errors.Wrap(err, "Couldn't get system_domain option")
	}

	var message string
	if c.HasIstio() {
		message = "Creating Tekton dashboard istio ingress gateway"
		_, err = helpers.WaitForCommandCompletion(ui, message,
			func() (string, error) {
				return helpers.CreateIstioIngressGateway("tekton", tektonNamespace, TektonDeploymentID+"."+domain, "tekton-dashboard", 9097)
			},
		)
	} else {
		message = "Creating Tekton dashboard ingress"
		_, err = helpers.WaitForCommandCompletion(ui, message,
			func() (string, error) {
				return "", createTektonIngress(c, TektonDeploymentID+"."+domain)
			},
		)
	}
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s failed", message))
	}

	ui.Success().Msg(fmt.Sprintf("Tekton deployed (http://tekton.%s).", domain))

	return nil
}

func (k Tekton) GetVersion() string {
	return fmt.Sprintf("pipelines: %s, triggers %s, dashboard: %s",
		tektonPipelineYamlPath, tektonTriggersYamlPath, tektonDashboardYamlPath)
}

func (k Tekton) Deploy(c *kubernetes.Cluster, ui *ui.UI, options kubernetes.InstallationOptions) error {

	_, err := c.Kubectl.CoreV1().Namespaces().Get(
		context.Background(),
		TektonDeploymentID,
		metav1.GetOptions{},
	)
	if err == nil {
		ui.Exclamation().Msg("Namespace " + TektonDeploymentID + " already present, skipping installation")
		return nil
	}

	ui.Note().KeeplineUnder(1).Msg("Deploying Tekton...")

	err = k.apply(c, ui, options, false)
	if err != nil {
		return err
	}

	return nil
}

func (k Tekton) Upgrade(c *kubernetes.Cluster, ui *ui.UI, options kubernetes.InstallationOptions) error {
	_, err := c.Kubectl.CoreV1().Namespaces().Get(
		context.Background(),
		TektonDeploymentID,
		metav1.GetOptions{},
	)
	if err != nil {
		return errors.New("Namespace " + TektonDeploymentID + " not present")
	}

	ui.Note().Msg("Upgrading Tekton...")

	return k.apply(c, ui, options, true)
}

func createTektonIngress(c *kubernetes.Cluster, subdomain string) error {
	_, err := c.Kubectl.ExtensionsV1beta1().Ingresses("tekton-pipelines").Create(
		context.Background(),
		// TODO: Switch to networking v1 when we don't care about <1.18 clusters
		// Like this (which has been reverted):
		// https://github.com/SUSE/carrier/commit/7721d610fdf27a79be980af522783671d3ffc198
		&v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tekton-dashboard",
				Namespace: "tekton-pipelines",
				Annotations: map[string]string{
					"kubernetes.io/ingress.class": "traefik",
				},
			},
			Spec: v1beta1.IngressSpec{
				Rules: []v1beta1.IngressRule{
					{
						Host: subdomain,
						IngressRuleValue: v1beta1.IngressRuleValue{
							HTTP: &v1beta1.HTTPIngressRuleValue{
								Paths: []v1beta1.HTTPIngressPath{
									{
										Path: "/",
										Backend: v1beta1.IngressBackend{
											ServiceName: "tekton-dashboard",
											ServicePort: intstr.IntOrString{
												Type:   intstr.Int,
												IntVal: 9097,
											},
										}}}}}}}}},
		metav1.CreateOptions{},
	)

	return err
}
