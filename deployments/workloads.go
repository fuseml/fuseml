package deployments

import (
	"context"
	"fmt"

	"github.com/fuseml/fuseml/cli/helpers"
	"github.com/fuseml/fuseml/cli/kubernetes"
	"github.com/fuseml/fuseml/cli/paas/ui"
	"github.com/kyokomi/emoji"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Workloads struct {
	Debug   bool
	Timeout int
}

const (
	WorkloadsDeploymentID   = "fuseml-workloads"
	WorkloadsIngressVersion = "0.1"
	appIngressYamlPath      = "app-ingress.yaml"
)

var roleRules = []rbacv1.PolicyRule{{
	APIGroups: []string{""},
	Resources: []string{"secrets", "serviceaccounts"},
	Verbs:     []string{"get", "create", "patch"},
}, {
	APIGroups: []string{"apps"},
	Resources: []string{"deployments"},
	Verbs:     []string{"get", "list", "watch"},
}}

func (k *Workloads) ID() string {
	return WorkloadsDeploymentID
}

func (k *Workloads) Backup(c *kubernetes.Cluster, ui *ui.UI, d string) error {
	return nil
}

func (k *Workloads) Restore(c *kubernetes.Cluster, ui *ui.UI, d string) error {
	return nil
}

func (k Workloads) Describe() string {
	return emoji.Sprintf(":cloud:Workloads Eirinix Ingress Version: %s\n", WorkloadsIngressVersion)
}

// Delete removes Workloads from kubernetes cluster
func (w Workloads) Delete(c *kubernetes.Cluster, ui *ui.UI) error {
	ui.Note().KeeplineUnder(1).Msg("Removing Workloads...")

	existsAndOwned, err := c.NamespaceExistsAndOwned(WorkloadsDeploymentID)
	if err != nil {
		return errors.Wrapf(err, "failed to check if namespace '%s' is owned or not", WorkloadsDeploymentID)
	}
	if !existsAndOwned {
		ui.Exclamation().Msg("Skipping Workspace because namespace either doesn't exist or not owned by Fuseml")
		return nil
	}

	if err := w.deleteWorkloadsNamespace(c, ui); err != nil {
		return errors.Wrapf(err, "Failed deleting namespace %s", WorkloadsDeploymentID)
	}

	existsAndOwned, err = c.NamespaceExistsAndOwned("app-ingress")
	if err != nil {
		return errors.Wrapf(err, "failed to check if namespace 'app-ingress' is owned or not")
	}
	if !existsAndOwned {
		ui.Exclamation().Msg("Skipping app-ingress namespace deletion because either doesn't exist or not owned by Fuseml")
		return nil
	}

	if out, err := helpers.KubectlDeleteEmbeddedYaml(appIngressYamlPath, true); err != nil {
		return errors.Wrap(err, fmt.Sprintf("Deleting %s failed:\n%s", appIngressYamlPath, out))
	}

	ui.Success().Msg("Workloads removed")

	return nil
}

func (w Workloads) apply(c *kubernetes.Cluster, ui *ui.UI, options kubernetes.InstallationOptions) error {
	if err := w.createWorkloadsNamespace(c, ui, options); err != nil {
		return err
	}

	if !c.HasIstio() {
		if out, err := helpers.KubectlApplyEmbeddedYaml(appIngressYamlPath); err != nil {
			return errors.Wrap(err, fmt.Sprintf("Installing %s failed:\n%s", appIngressYamlPath, out))
		}

		if err := c.LabelNamespace("app-ingress", kubernetes.FusemlDeploymentLabelKey, kubernetes.FusemlDeploymentLabelValue); err != nil {
			return err
		}

		if err := c.WaitUntilPodBySelectorExist(ui, "app-ingress", "name=app-ingress", w.Timeout); err != nil {
			return errors.Wrap(err, "failed waiting app-ingress deployment to exist")
		}
	}

	ui.Success().Msg("Workloads deployed")

	return nil
}

func (k Workloads) GetVersion() string {
	// TODO: Maybe this should be the Fuseml version itself?
	return WorkloadsIngressVersion
}

func (k Workloads) Deploy(c *kubernetes.Cluster, ui *ui.UI, options kubernetes.InstallationOptions) error {
	_, err := c.Kubectl.CoreV1().Namespaces().Get(
		context.Background(),
		WorkloadsDeploymentID,
		metav1.GetOptions{},
	)
	if err == nil {
		ui.Exclamation().Msg("Namespace " + WorkloadsDeploymentID + " already present, skipping installation")
		return nil
	}

	ui.Note().KeeplineUnder(1).Msg("Deploying Workloads...")

	err = k.apply(c, ui, options)
	if err != nil {
		return err
	}

	return nil
}

func (k Workloads) Upgrade(c *kubernetes.Cluster, ui *ui.UI, options kubernetes.InstallationOptions) error {
	// NOTE: Not implemented yet
	return nil
}

func (w Workloads) createWorkloadsNamespace(c *kubernetes.Cluster, ui *ui.UI, options kubernetes.InstallationOptions) error {
	if _, err := c.Kubectl.CoreV1().Namespaces().Create(
		context.Background(),
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   WorkloadsDeploymentID,
				Labels: map[string]string{kubernetes.FusemlDeploymentLabelKey: kubernetes.FusemlDeploymentLabelValue},
			},
		},
		metav1.CreateOptions{},
	); err != nil {
		return nil
	}

	if err := c.LabelNamespace(WorkloadsDeploymentID, kubernetes.FusemlDeploymentLabelKey, kubernetes.FusemlDeploymentLabelValue); err != nil {
		return err
	}
	if err := w.createGiteaCredsSecret(c, options); err != nil {
		return err
	}
	if err := w.createWorkloadsServiceAccountWithSecretAccess(c); err != nil {
		return err
	}
	if err := w.createWorkloadsRole(c); err != nil {
		return err
	}
	if err := w.createWorkloadsRoleBinding(c); err != nil {
		return err
	}

	return nil
}

func (w Workloads) deleteWorkloadsNamespace(c *kubernetes.Cluster, ui *ui.UI) error {
	message := "Deleting Workloads namespace " + WorkloadsDeploymentID
	_, err := helpers.WaitForCommandCompletion(ui, message,
		func() (string, error) {
			return "", c.DeleteNamespace(WorkloadsDeploymentID)
		},
	)
	if err != nil {
		return err
	}

	message = "Waiting for workloads namespace to be gone"
	warning, err := helpers.WaitForCommandCompletion(ui, message,
		func() (string, error) {
			var err error
			for err == nil {
				_, err = c.Kubectl.CoreV1().Namespaces().Get(
					context.Background(),
					WorkloadsDeploymentID,
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

	return nil
}

func (w Workloads) createGiteaCredsSecret(c *kubernetes.Cluster, options kubernetes.InstallationOptions) error {
	domain, err := options.GetString("system_domain", GiteaDeploymentID)
	if err != nil {
		return err
	}
	giteaSubdomain := GiteaDeploymentID + "." + domain
	_, err = c.Kubectl.CoreV1().Secrets(WorkloadsDeploymentID).Create(context.Background(),
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "gitea-creds",
				Annotations: map[string]string{
					"tekton.dev/git-0": fmt.Sprintf("http://%s", giteaSubdomain),
				},
			},
			StringData: map[string]string{
				"username": "dev",
				"password": "changeme",
			},
			Type: "kubernetes.io/basic-auth",
		}, metav1.CreateOptions{})

	if err != nil {
		return err
	}
	return nil
}

func (w Workloads) createWorkloadsServiceAccountWithSecretAccess(c *kubernetes.Cluster) error {
	automountServiceAccountToken := true
	_, err := c.Kubectl.CoreV1().ServiceAccounts(WorkloadsDeploymentID).Create(
		context.Background(),
		&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name: WorkloadsDeploymentID,
			},
			Secrets:                      []corev1.ObjectReference{{Name: "gitea-creds"}},
			AutomountServiceAccountToken: &automountServiceAccountToken,
		}, metav1.CreateOptions{})

	return err
}

func (w Workloads) createWorkloadsRole(c *kubernetes.Cluster) error {
	_, err := c.Kubectl.RbacV1().Roles(WorkloadsDeploymentID).Create(
		context.Background(),
		&rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name: WorkloadsDeploymentID,
			},
			Rules: roleRules,
		}, metav1.CreateOptions{})
	return err
}

// Update the role with new rule
func (w Workloads) updateWorkloadsRole(c *kubernetes.Cluster, newRule rbacv1.PolicyRule) error {

	role, err := c.Kubectl.RbacV1().Roles(WorkloadsDeploymentID).Get(
		context.Background(),
		WorkloadsDeploymentID,
		metav1.GetOptions{},
	)

	for _, rule := range role.Rules {
		// this is only a simple check for exact duplicates; we ignore the situation
		// when e.g. the new rule is subset of existing one
		if helpers.StringSlicesEqual(rule.APIGroups, newRule.APIGroups) &&
			helpers.StringSlicesEqual(rule.Resources, newRule.Resources) &&
			helpers.StringSlicesEqual(rule.Verbs, newRule.Verbs) {
			return nil
		}
	}

	role, err = c.Kubectl.RbacV1().Roles(WorkloadsDeploymentID).Update(
		context.Background(),
		&rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name: WorkloadsDeploymentID,
			},
			Rules: append(role.Rules, newRule),
		}, metav1.UpdateOptions{})

	return err
}

func (w Workloads) createWorkloadsRoleBinding(c *kubernetes.Cluster) error {
	_, err := c.Kubectl.RbacV1().RoleBindings(WorkloadsDeploymentID).Create(
		context.Background(),
		&rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: WorkloadsDeploymentID,
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Role",
				Name:     WorkloadsDeploymentID,
			},
			Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: WorkloadsDeploymentID, Namespace: WorkloadsDeploymentID}},
		}, metav1.CreateOptions{})
	return err
}
