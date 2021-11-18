package deployments

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

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
	TektonDeploymentID            = "tekton-pipelines"
	tektonOperatorNamespace       = "tekton-operator"
	tektonOperatorYamlPath        = "tekton/install/operator.yaml"
	tektonOperatorProfileYamlPath = "tekton/install/profile-all.yaml"
	tektonTriggersSAYamlPath      = "tekton/install/sa.yaml"
	tektonFuseMLTasksYamlPath     = "tekton/tasks"
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
	return emoji.Sprintf(":cloud:Tekton Operator: %s\n:cloud:Tekton Operator Profile: %s\n",
		tektonOperatorYamlPath, tektonOperatorProfileYamlPath)
}

// Delete removes Tekton from kubernetes cluster
func (k Tekton) Delete(c *kubernetes.Cluster, ui *ui.UI) error {
	ui.Note().KeeplineUnder(1).Msg("Removing Tekton...")

	wasDeleted := false
	namespaces := []string{TektonDeploymentID, tektonOperatorNamespace}

	for _, ns := range namespaces {
		existsAndOwned, err := c.NamespaceExistsAndOwned(ns)
		if err != nil {
			return errors.Wrapf(err, "failed to check if namespace '%s' is owned or not", ns)
		}
		if !existsAndOwned {
			ui.Exclamation().Msg(fmt.Sprintf("Skipping %s because namespace either doesn't exist or not owned by FuseML", ns))
		} else {
			var yamlFile string
			if ns == TektonDeploymentID {
				yamlFile = tektonOperatorProfileYamlPath
			} else {
				yamlFile = tektonOperatorYamlPath
			}
			if out, err := helpers.KubectlDeleteEmbeddedYaml(yamlFile, true); err != nil {
				return errors.Wrap(err, fmt.Sprintf("Deleting %s failed:\n%s", yamlFile, out))
			}
			// wait for sa, roles and tekton-[pipelines|triggers|dashboard] to be deleted before deleting operator
			if ns == TektonDeploymentID {
				message := "Deleting Tekton triggers Service Account and Roles"
				out, err := helpers.WaitForCommandCompletion(ui, message,
					func() (string, error) {
						return helpers.KubectlDeleteEmbeddedYaml(tektonTriggersSAYamlPath, true)
					},
				)
				if err != nil {
					return errors.Wrap(err, fmt.Sprintf("%s failed:\n%s", message, out))
				}

				message = "Waiting for tekton to be deleted"
				out, err = helpers.WaitForCommandCompletion(ui, message,
					func() (string, error) {
						return helpers.Kubectl(fmt.Sprintf("wait --for=delete --timeout=%ds -n %s tektonconfig/config",
							k.Timeout, TektonDeploymentID))
					},
				)
				if err != nil && strings.HasSuffix(out, "not found") {
					return errors.Wrap(err, fmt.Sprintf("%s failed:\n%s", message, out))
				}
			}

			message := fmt.Sprintf("Deleting %s namespace", ns)
			_, err = helpers.WaitForCommandCompletion(ui, message,
				func() (string, error) {
					return "", c.DeleteNamespace(ns)
				},
			)
			if err != nil {
				return errors.Wrapf(err, "Failed deleting namespace %s", ns)
			}
			wasDeleted = true
		}
	}

	if wasDeleted {
		ui.Success().Msg("Tekton removed")
	}

	return nil
}

func (k Tekton) apply(c *kubernetes.Cluster, ui *ui.UI, options kubernetes.InstallationOptions, upgrade bool) error {

	installOperator := true
	if !upgrade {
		// Install tekton operator only if not already installed
		_, err := c.Kubectl.CoreV1().Namespaces().Get(
			context.Background(),
			tektonOperatorNamespace,
			metav1.GetOptions{},
		)
		if err == nil {
			ui.Exclamation().Msg("Tekton Operator already present, skipping its installation")
			installOperator = false
		}
	}

	if installOperator || upgrade {
		if out, err := helpers.KubectlApplyEmbeddedYaml(tektonOperatorYamlPath); err != nil {
			return errors.Wrap(err, fmt.Sprintf("installing %s failed:\n%s", tektonOperatorYamlPath, out))
		}

		err := c.LabelNamespace(tektonOperatorNamespace, kubernetes.FusemlDeploymentLabelKey, kubernetes.FusemlDeploymentLabelValue)
		if err != nil {
			return err
		}

		for _, crd := range []string{
			"tektonconfigs.operator.tekton.dev",
			"tektondashboards.operator.tekton.dev",
			"tektoninstallersets.operator.tekton.dev",
			"tektonpipelines.operator.tekton.dev",
			"tektonresults.operator.tekton.dev",
			"tektontriggers.operator.tekton.dev",
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

		message := "Waiting for tekton-operator pod to be ready"
		out, err := helpers.WaitForCommandCompletion(ui, message,
			func() (string, error) {
				return helpers.Kubectl(fmt.Sprintf("wait --for=condition=Ready --timeout=%ds -n %s --selector=app=tekton-operator pod",
					k.Timeout, tektonOperatorNamespace))
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

	if !upgrade {
		attempts := 3
		sleep := 3 * time.Second
		for i := 0; i < attempts; i++ {
			if i > 0 {
				time.Sleep(sleep)
				sleep *= 2
			}
			if out, err := helpers.KubectlApplyEmbeddedYaml(tektonOperatorProfileYamlPath); i == (attempts-1) && err != nil {
				return errors.Wrap(err, fmt.Sprintf("installing %s failed:\n%s", tektonOperatorProfileYamlPath, out))
			}
		}

		out, err := helpers.WaitForKubernetesResourceToExist(ui, TektonDeploymentID, "namespace", TektonDeploymentID, k.Timeout)
		if err != nil {
			return fmt.Errorf("error waiting for namespace %s: %s", TektonDeploymentID, out)
		}

		err = c.LabelNamespace(TektonDeploymentID, kubernetes.FusemlDeploymentLabelKey, kubernetes.FusemlDeploymentLabelValue)
		if err != nil {
			return err
		}

		var message string
		if c.HasIstio() {
			message = "Creating Tekton dashboard istio ingress gateway"
			_, err = helpers.WaitForCommandCompletion(ui, message,
				func() (string, error) {
					return helpers.CreateIstioIngressGateway("tekton", TektonDeploymentID, "tekton."+domain, "tekton-dashboard", 9097)
				},
			)
		} else {
			message = "Creating Tekton dashboard ingress"
			_, err = helpers.WaitForCommandCompletion(ui, message,
				func() (string, error) {
					return "", createTektonIngress(c, "tekton."+domain)
				},
			)
		}
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("%s failed", message))
		}
	}

	message := "Waiting for tekton to be ready"
	out, err := helpers.WaitForCommandCompletion(ui, message,
		func() (string, error) {
			return helpers.Kubectl(fmt.Sprintf("wait --for=condition=Ready --timeout=%ds -n %s tektonconfig/config",
				k.Timeout, TektonDeploymentID))
		},
	)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s failed:\n%s", message, out))
	}

	message = "Installing Tekton triggers Service Account"
	out, err = helpers.WaitForCommandCompletion(ui, message,
		func() (string, error) {
			return helpers.KubectlApplyEmbeddedYaml(tektonTriggersSAYamlPath)
		},
	)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s failed:\n%s", message, out))
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

	ui.Success().Msg(fmt.Sprintf("Tekton deployed (http://tekton.%s).", domain))

	return nil
}

func (k Tekton) GetVersion() string {
	versions := map[string]string{}
	for _, c := range []string{"pipeline", "trigger", "dashboard"} {
		version, err := helpers.Kubectl(fmt.Sprintf("get tekton%ss %s -o jsonpath='{.status.version}')", c, c))
		if err != nil {
			versions[c] = "Unknown"
		} else {
			versions[c] = version
		}
	}

	return fmt.Sprintf("pipelines: %s, triggers: %s, dashboard: %s",
		versions["pipeline"], versions["trigger"], versions["dashboard"])
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
	_, err := c.Kubectl.ExtensionsV1beta1().Ingresses(TektonDeploymentID).Create(
		context.Background(),
		// TODO: Switch to networking v1 when we don't care about <1.18 clusters
		// Like this (which has been reverted):
		// https://github.com/SUSE/carrier/commit/7721d610fdf27a79be980af522783671d3ffc198
		&v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tekton-dashboard",
				Namespace: TektonDeploymentID,
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
