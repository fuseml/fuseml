package paas

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"time"

	"code.gitea.io/sdk/gitea"
	"github.com/fuseml/fuseml/cli/helpers"
	"github.com/fuseml/fuseml/cli/kubernetes"
	"github.com/fuseml/fuseml/cli/kubernetes/tailer"
	"github.com/fuseml/fuseml/cli/paas/config"
	paasgitea "github.com/fuseml/fuseml/cli/paas/gitea"
	"github.com/fuseml/fuseml/cli/paas/ui"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	knversionedclient "knative.dev/serving/pkg/client/clientset/versioned"
)

// FusemlClient provides functionality for talking to a
// Fuseml installation on Kubernetes
type FusemlClient struct {
	giteaClient   *gitea.Client
	kubeClient    *kubernetes.Cluster
	ui            *ui.UI
	config        *config.Config
	giteaResolver *paasgitea.Resolver
	Log           logr.Logger
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

	giteaVersion := "unavailable"

	version, resp, err := c.giteaClient.ServerVersion()
	if err == nil && resp != nil && resp.StatusCode == 200 {
		giteaVersion = version
	}

	c.ui.Success().
		WithStringValue("Platform", platform.String()).
		WithStringValue("Kubernetes Version", kubeVersion).
		WithStringValue("Gitea Version", giteaVersion).
		Msg("Fuseml Environment")

	return nil
}

// AppsMatching returns all Fuseml apps having the specified prefix
// in their name.
func (c *FusemlClient) AppsMatching(prefix string) []string {
	log := c.Log.WithName("AppsMatching").WithValues("PrefixToMatch", prefix)
	log.Info("start")
	defer log.Info("return")
	details := log.V(1) // NOTE: Increment of level, not absolute.

	result := []string{}

	apps, _, err := c.giteaClient.ListOrgRepos(c.config.Org, gitea.ListOrgReposOptions{})
	if err != nil {
		return result
	}

	for _, app := range apps {
		details.Info("Found", "Name", app.Name)

		if strings.HasPrefix(app.Name, prefix) {
			details.Info("Matched", "Name", app.Name)
			result = append(result, app.Name)
		}
	}

	return result
}

// Apps gets all Fuseml apps
func (c *FusemlClient) Apps() error {
	log := c.Log.WithName("Apps").WithValues("Organization", c.config.Org)
	log.Info("start")
	defer log.Info("return")
	details := log.V(1) // NOTE: Increment of level, not absolute.

	c.ui.Note().
		WithStringValue("Organization", c.config.Org).
		Msg("Listing applications")

	details.Info("validate")
	err := c.ensureGoodOrg(c.config.Org, "Unable to list applications.")
	if err != nil {
		return err
	}

	details.Info("gitea list org repos")
	apps, _, err := c.giteaClient.ListOrgRepos(c.config.Org, gitea.ListOrgReposOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to list apps")
	}

	msg := c.ui.Success().WithTable("Name", "Status", "Routes")

	for _, app := range apps {
		details.Info("kube get status", "App", app.Name)
		status, err := c.kubeClient.DeploymentStatus(
			c.config.FusemlWorkloadsNamespace,
			fmt.Sprintf("fuseml/app-guid=%s.%s", c.config.Org, app.Name),
		)
		if err != nil {
			return errors.Wrapf(err, "failed to get status for app '%s'", app.Name)
		}

		var routes string
		if c.kubeClient.HasKnative() {
			details.Info("kube get knative services", "App", app.Name)

			knc, err := knversionedclient.NewForConfig(c.kubeClient.RestConfig)
			if err != nil {
				return errors.Wrap(err, "failed to create knative client.")
			}

			knService, err := knc.ServingV1().Services(c.config.FusemlWorkloadsNamespace).
				List(context.TODO(), metav1.ListOptions{LabelSelector: fmt.Sprintf("fuseml/app-guid=%s.%s", c.config.Org, app.Name)})
			if err != nil {
				return errors.Wrap(err, "failed to get knative service")
			}
			if len(knService.Items) < 1 {
				// knative is deployed, but maybe not used for current app - show the default route instead
				defaultRoute, err := c.appDefaultRoute(app.Name)
				if err != nil {
					return errors.Wrapf(err, "failed to get for app '%s'", app.Name)
				}
				routes = "http://" + defaultRoute
			} else {
				// FIXME: KN services created by KFServing has -predictor-default appended into its URL, this code is hardcoded to replace it for now
				// but needs a better approach for this
				routes = strings.ReplaceAll(knService.Items[0].Status.URL.String(), "-predictor-default.", ".")
			}
		} else if c.kubeClient.HasIstio() {
			defaultRoute, err := c.appDefaultRoute(app.Name)
			if err != nil {
				return errors.Wrapf(err, "failed to get routes for app '%s'", app.Name)
			}
			routes = "http://" + defaultRoute
		} else {
			details.Info("kube get ingress", "App", app.Name)
			ingRoutes, err := c.kubeClient.ListIngressRoutes(
				c.config.FusemlWorkloadsNamespace,
				app.Name)
			if err != nil {
				return errors.Wrapf(err, "failed to get routes for app '%s'", app.Name)
			}
			routes = "https://" + strings.Join(ingRoutes, ", ")
		}

		inferenceUrl, err := c.getAppInferenceUrl(app.Name)
		if err != nil {
			return err
		}
		routes = fmt.Sprintf("%s/%s", routes, inferenceUrl)

		msg = msg.WithTableRow(app.Name, status, routes)
	}

	msg.Msg("Fuseml Applications:")

	return nil
}

// Delete deletes an app
func (c *FusemlClient) Delete(app string) error {
	log := c.Log.WithName("Delete").WithValues("Application", app)
	log.Info("start")
	defer log.Info("return")
	details := log.V(1) // NOTE: Increment of level, not absolute.

	c.ui.Note().
		WithStringValue("Name", app).
		Msg("Deleting application...")

	appDir, err := c.gitCloneApp(app)
	if err != nil {
		return errors.Wrap(err, "failed cloning app repository")
	}
	defer os.RemoveAll(appDir)

	details.Info("deleting app workload")
	out, err := helpers.Kubectl(fmt.Sprintf("delete -n %s --filename %s/.fuseml/serve.yaml", c.config.FusemlWorkloadsNamespace, appDir))
	if err != nil {
		return errors.Wrap(err, `failed to delete application deployment`+out)
	}
	c.ui.Normal().Msg("Deleted app workload.")

	details.Info("delete repo")
	_, err = c.giteaClient.DeleteRepo(c.config.Org, app)
	if err != nil {
		return errors.Wrap(err, "failed to delete repo")
	}
	c.ui.Normal().Msg("Deleted app code repository.")

	c.ui.Success().Msg("Application deleted.")

	return nil
}

func (c *FusemlClient) appDefaultRoute(name string) (string, error) {
	domain, err := c.giteaResolver.GetMainDomain()
	if err != nil {
		return "", errors.Wrap(err, "failed to determine fuseml domain")
	}
	route := fmt.Sprintf("%s.%s", name, domain)

	if c.kubeClient.HasIstio() {
		route = fmt.Sprintf("%s-%s.%s.%s", c.config.Org, name, c.config.FusemlWorkloadsNamespace, domain)
	}

	return route, nil
}

func (c *FusemlClient) logs(name string) (context.CancelFunc, error) {
	c.ui.ProgressNote().V(1).Msg("Tailing application logs ...")

	ctx, cancelFunc := context.WithCancel(context.Background())

	// TODO: improve the way we look for pods, use selectors
	// and watch staging as well
	err := tailer.Run(c.ui, ctx, &tailer.Config{
		ContainerQuery:        regexp.MustCompile(".*"),
		ExcludeContainerQuery: nil,
		ContainerState:        "running",
		Exclude:               nil,
		Include:               nil,
		Timestamps:            false,
		Since:                 48 * time.Hour,
		AllNamespaces:         false,
		LabelSelector:         labels.Everything(),
		TailLines:             nil,
		Template:              tailer.DefaultSingleNamespaceTemplate(),

		Namespace: "fuseml-workloads",
		PodQuery:  regexp.MustCompile(fmt.Sprintf(".*-%s-.*", name)),
	}, c.kubeClient)
	if err != nil {
		return cancelFunc, errors.Wrap(err, "failed to start log tail")
	}

	return cancelFunc, nil
}

func (c *FusemlClient) ensureGoodOrg(org, msg string) error {
	_, resp, err := c.giteaClient.GetOrg(org)
	if resp == nil && err != nil {
		return errors.Wrap(err, "failed to make get org request")
	}

	if resp.StatusCode == 404 {
		errmsg := "Organization does not exist."
		if msg != "" {
			errmsg += " " + msg
		}
		c.ui.Exclamation().WithEnd(1).Msg(errmsg)
	}

	return nil
}

func (c *FusemlClient) getServingWorkloadType(serve string) string {
	if serve != "" {
		return strings.ToLower(serve)
	}
	if c.kubeClient.HasIstio() {
		return "knative"
	}
	return "deployment"
}

func (c *FusemlClient) gitCloneApp(name string) (string, error) {
	c.ui.Normal().Msg("Cloning application code ...")

	tmpDir, err := ioutil.TempDir("", "fuseml-app")
	if err != nil {
		return "", errors.Wrap(err, "can't create temp directory")
	}

	giteaURL, err := c.giteaResolver.GetGiteaURL()
	if err != nil {
		return "", errors.Wrap(err, "failed to resolve gitea host")
	}

	u, err := url.Parse(giteaURL)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse gitea url")
	}

	username, password, err := c.giteaResolver.GetGiteaCredentials()
	if err != nil {
		return "", errors.Wrap(err, "failed to resolve gitea credentials")
	}

	u.User = url.UserPassword(username, password)
	u.Path = path.Join(u.Path, c.config.Org, name)

	cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf(`
cd "%s"
git clone --depth 1 "%s" .
`, tmpDir, u.String()))

	output, err := cmd.CombinedOutput()
	if err != nil {
		c.ui.Problem().
			WithStringValue("Stdout", string(output)).
			WithStringValue("Stderr", "").
			Msg("App push failed")
		return "", errors.Wrap(err, "push script failed")
	}

	c.ui.Note().V(1).WithStringValue("Output", string(output)).Msg("")
	c.ui.Success().Msg("Application clone successful")
	return tmpDir, nil
}

func (c *FusemlClient) getAppInferenceUrl(appName string) (string, error) {
	appDeployment, err := c.kubeClient.Kubectl.AppsV1().Deployments(c.config.FusemlWorkloadsNamespace).
		List(context.TODO(), metav1.ListOptions{LabelSelector: fmt.Sprintf("fuseml/app-guid=%s.%s", c.config.Org, appName)})
	if err != nil {
		return "", errors.Wrapf(err, "failed to get inference url for app '%s'", appName)
	}
	if len(appDeployment.Items) == 0 {
		return "", errors.New(fmt.Sprintf("No deployment of application %s.%s found", c.config.Org, appName))
	}

	// Labels have the limitations of 63 characters, to overcome that use '-NAME-' on the label to represent the deployment name
	// that is also used on the URL for some inference services.
	inferUrl := strings.ReplaceAll(appDeployment.Items[0].Labels["fuseml/infer-url"], "-NAME-", fmt.Sprintf("%s-%s", c.config.Org, appName))

	// Labels does not allow '/' characters, in that way we are replacing '/' with '_' on the template, so
	// we need to replace '_' back to '/' here. This is not a solution, but a temporary workaround that will break
	// as soon as a url has '_' on it.
	return strings.ReplaceAll(inferUrl, "_", "/"), nil
}
