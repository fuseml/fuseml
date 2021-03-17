package paas

import (
	"context"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"code.gitea.io/sdk/gitea"
	"github.com/fuseml/fuseml/cli/deployments"
	"github.com/fuseml/fuseml/cli/helpers"
	"github.com/fuseml/fuseml/cli/kubernetes"
	"github.com/fuseml/fuseml/cli/kubernetes/tailer"
	"github.com/fuseml/fuseml/cli/paas/config"
	paasgitea "github.com/fuseml/fuseml/cli/paas/gitea"
	"github.com/fuseml/fuseml/cli/paas/ui"
	"github.com/go-logr/logr"
	"github.com/otiai10/copy"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	knversionedclient "knative.dev/serving/pkg/client/clientset/versioned"
)

var (
	// HookSecret should be generated
	// TODO: generate this and put it in a secret
	HookSecret = "74tZTBHkhjMT5Klj6Ik6PqmM"

	// StagingEventListenerURL should not exist
	// TODO: detect this based on namespaces and services
	StagingEventListenerURL = "http://el-mlflow-listener.fuseml-workloads:8080"
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
				routes = defaultRoute
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
			routes = defaultRoute
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

// CreateOrg creates an Org in gitea
func (c *FusemlClient) CreateOrg(org string) error {
	log := c.Log.WithName("CreateOrg").WithValues("Organization", org)
	log.Info("start")
	defer log.Info("return")
	details := log.V(1) // NOTE: Increment of level, not absolute.

	c.ui.Note().
		WithStringValue("Name", org).
		Msg("Creating organization...")

	details.Info("validate")
	details.Info("gitea get-org")
	_, resp, err := c.giteaClient.GetOrg(org)
	if resp == nil && err != nil {
		return errors.Wrap(err, "failed to make get org request")
	}

	if resp.StatusCode == 200 {
		c.ui.Exclamation().Msg("Organization already exists.")
		return nil
	}

	details.Info("gitea create-org")
	_, _, err = c.giteaClient.CreateOrg(gitea.CreateOrgOption{
		Name: org,
	})

	if err != nil {
		return errors.Wrap(err, "failed to create org")
	}

	c.ui.Success().Msg("Organization created.")

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

// OrgsMatching returns all Fuseml orgs having the specified prefix
// in their name
func (c *FusemlClient) OrgsMatching(prefix string) []string {
	log := c.Log.WithName("OrgsMatching").WithValues("PrefixToMatch", prefix)
	log.Info("start")
	defer log.Info("return")
	details := log.V(1) // NOTE: Increment of level, not absolute.

	result := []string{}

	orgs, _, err := c.giteaClient.AdminListOrgs(gitea.AdminListOrgsOptions{})
	if err != nil {
		return result
	}

	for _, org := range orgs {
		details.Info("Found", "Name", org.UserName)

		if strings.HasPrefix(org.UserName, prefix) {
			details.Info("Matched", "Name", org.UserName)
			result = append(result, org.UserName)
		}
	}

	return result
}

// Orgs get a list of all orgs in gitea
func (c *FusemlClient) Orgs() error {
	log := c.Log.WithName("Orgs")
	log.Info("start")
	defer log.Info("return")
	details := log.V(1) // NOTE: Increment of level, not absolute.

	c.ui.Note().Msg("Listing organizations")

	details.Info("gitea admin list orgs")
	orgs, _, err := c.giteaClient.AdminListOrgs(gitea.AdminListOrgsOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to list orgs")
	}

	msg := c.ui.Success().WithTable("Name")

	for _, org := range orgs {
		msg = msg.WithTableRow(org.UserName)
	}

	msg.Msg("Fuseml Organizations:")

	return nil
}

// Push pushes an app
func (c *FusemlClient) Push(app string, path string, serve string) error {
	log := c.Log.
		WithName("Push").
		WithValues("Name", app,
			"Organization", c.config.Org,
			"Sources", path)
	log.Info("start")
	defer log.Info("return")
	details := log.V(1) // NOTE: Increment of level, not absolute.

	c.ui.Note().
		WithStringValue("Name", app).
		WithStringValue("Sources", path).
		WithStringValue("Organization", c.config.Org).
		Msg("About to push an application with given name and sources into the specified organization")

	c.ui.Exclamation().
		Timeout(5 * time.Second).
		Msg("Hit Enter to continue or Ctrl+C to abort (deployment will continue automatically in 5 seconds)")

	details.Info("validate")
	err := c.ensureGoodOrg(c.config.Org, "Unable to push.")
	if err != nil {
		return err
	}

	details.Info("create repo")
	err = c.createRepo(app)
	if err != nil {
		return errors.Wrap(err, "create repo failed")
	}

	details.Info("create repo webhook")
	err = c.createRepoWebhook(app)
	if err != nil {
		return errors.Wrap(err, "webhook configuration failed")
	}

	details.Info("prepare code")
	tmpDir, err := c.prepareCode(app, c.config.Org, path, serve)
	if err != nil {
		return errors.Wrap(err, "failed to prepare code")
	}

	details.Info("git push")
	err = c.gitPush(app, tmpDir)
	if err != nil {
		return errors.Wrap(err, "failed to git push code")
	}

	details.Info("start tailing logs")
	stopFunc, err := c.logs(app)
	if err != nil {
		return errors.Wrap(err, "failed to tail logs")
	}
	defer stopFunc()

	details.Info("wait for apps")
	err = c.waitForApp(c.config.Org, app)
	if err != nil {
		return errors.Wrap(err, "waiting for app failed")
	}

	details.Info("get app default route")
	route, err := c.appDefaultRoute(app)
	if err != nil {
		return errors.Wrap(err, "failed to determine default app route")
	}

	details.Info("get app inference url")
	inferenceUrl, err := c.getAppInferenceUrl(app)
	if err != nil {
		return errors.Wrap(err, "failed to determine app inference URL")
	}

	protocol := "http"
	if !c.kubeClient.HasIstio() {
		protocol = "https"
	}

	c.ui.Success().
		WithStringValue("Name", app).
		WithStringValue("Organization", c.config.Org).
		WithStringValue("Route", fmt.Sprintf("%s://%s/%s", protocol, route, inferenceUrl)).
		Msg("App is online.")

	return nil
}

// Target targets an org in gitea
func (c *FusemlClient) Target(org string) error {
	log := c.Log.WithName("Target").WithValues("Organization", org)
	log.Info("start")
	defer log.Info("return")
	details := log.V(1) // NOTE: Increment of level, not absolute.

	if org == "" {
		details.Info("query config")
		c.ui.Success().
			WithStringValue("Currently targeted organization", c.config.Org).
			Msg("")
		return nil
	}

	c.ui.Note().
		WithStringValue("Name", org).
		Msg("Targeting organization...")

	details.Info("validate")
	err := c.ensureGoodOrg(org, "Unable to target.")
	if err != nil {
		return err
	}

	details.Info("set config")
	c.config.Org = org
	err = c.config.Save()
	if err != nil {
		return errors.Wrap(err, "failed to save configuration")
	}

	c.ui.Success().Msg("Organization targeted.")

	return nil
}

func (c *FusemlClient) check() {
	c.giteaClient.GetMyUserInfo()
}

func (c *FusemlClient) createRepo(name string) error {
	_, resp, err := c.giteaClient.GetRepo(c.config.Org, name)
	if resp == nil && err != nil {
		return errors.Wrap(err, "failed to make get repo request")
	}

	if resp.StatusCode == 200 {
		c.ui.Note().Msg("Application already exists. Updating.")
		return nil
	}

	_, _, err = c.giteaClient.CreateOrgRepo(c.config.Org, gitea.CreateRepoOption{
		Name:          name,
		AutoInit:      true,
		Private:       true,
		DefaultBranch: "main",
	})

	if err != nil {
		return errors.Wrap(err, "failed to create application")
	}

	c.ui.Success().Msg("Application Repository created.")

	return nil
}

func (c *FusemlClient) createRepoWebhook(name string) error {
	hooks, _, err := c.giteaClient.ListRepoHooks(c.config.Org, name, gitea.ListHooksOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to list webhooks")
	}

	for _, hook := range hooks {
		url := hook.Config["url"]
		if url == StagingEventListenerURL {
			c.ui.Normal().Msg("Webhook already exists.")
			return nil
		}
	}

	c.ui.Normal().Msg("Creating webhook in the repo...")

	c.giteaClient.CreateRepoHook(c.config.Org, name, gitea.CreateHookOption{
		Active:       true,
		BranchFilter: "*",
		Config: map[string]string{
			"secret":       HookSecret,
			"http_method":  "POST",
			"url":          StagingEventListenerURL,
			"content_type": "json",
		},
		Type: "gitea",
	})

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

func (c *FusemlClient) prepareCode(name, org, appDir string, serve string) (string, error) {
	c.ui.Normal().Msg("Preparing code ...")

	tmpDir, err := ioutil.TempDir("", "fuseml-app")
	if err != nil {
		return "", errors.Wrap(err, "can't create temp directory")
	}

	err = copy.Copy(appDir, tmpDir)
	if err != nil {
		return "", errors.Wrap(err, "failed to copy app sources to temp location")
	}

	err = os.MkdirAll(filepath.Join(tmpDir, ".fuseml"), 0700)
	if err != nil {
		return "", errors.Wrap(err, "failed to setup kube resources directory in temp app location")
	}

	dockerfileDef := `
FROM ghcr.io/fuseml/mlflow:1.14.1

COPY conda.yaml /env/
RUN env=$(awk '/name:/ {print $2}' /env/conda.yaml) && \
	sed -i "s/base/$env/" /root/.bashrc

ENV BASH_ENV /root/.bashrc
RUN conda env create -f /env/conda.yaml
	`

	route, err := c.appDefaultRoute(name)
	if err != nil {
		return "", errors.Wrap(err, "failed to calculate default app route")
	}

	dockerFile, err := os.Create(filepath.Join(tmpDir, ".fuseml", "Dockerfile"))
	if err != nil {
		return "", errors.Wrap(err, "failed to create file for FuseML resource definitions")
	}
	defer func() { err = dockerFile.Close() }()

	_, err = dockerFile.WriteString(dockerfileDef)
	if err != nil {
		return "", errors.Wrap(err, "failed to write FuseML Dockerfile definition")
	}

	servingType := c.getServingWorkloadType(serve)

	tmplPathOnDisk, err := helpers.ExtractFile(`serving/` + servingType + `.yaml.tmpl`)
	if err != nil {
		return "", errors.New("Failed to extract embedded file: " + tmplPathOnDisk + " - " + err.Error())
	}
	defer os.Remove(tmplPathOnDisk)

	servingTmpl, err := template.ParseFiles(tmplPathOnDisk)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse serving template for model")
	}

	appFile, err := os.Create(filepath.Join(tmpDir, ".fuseml", "serve.yaml"))
	if err != nil {
		return "", errors.Wrap(err, "failed to create file for kube resource definitions")
	}
	defer func() { err = appFile.Close() }()

	err = servingTmpl.Execute(appFile, struct {
		AppName            string
		Route              string
		Org                string
		ServiceAccountName string
	}{
		AppName:            name,
		Route:              route,
		Org:                c.config.Org,
		ServiceAccountName: deployments.WorkloadsDeploymentID,
	})
	if err != nil {
		return "", errors.Wrap(err, "failed to render kube resource definition")
	}

	return tmpDir, nil
}

func (c *FusemlClient) gitPush(name, tmpDir string) error {
	c.ui.Normal().Msg("Pushing application code ...")

	giteaURL, err := c.giteaResolver.GetGiteaURL()
	if err != nil {
		return errors.Wrap(err, "failed to resolve gitea host")
	}

	u, err := url.Parse(giteaURL)
	if err != nil {
		return errors.Wrap(err, "failed to parse gitea url")
	}

	username, password, err := c.giteaResolver.GetGiteaCredentials()
	if err != nil {
		return errors.Wrap(err, "failed to resolve gitea credentials")
	}

	u.User = url.UserPassword(username, password)
	u.Path = path.Join(u.Path, c.config.Org, name)

	cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf(`
cd "%s" 
git init
git config user.name "Fuseml"
git config user.email ci@fuseml
git remote add fuseml "%s"
git fetch --all
git reset --soft fuseml/main
git add --all
git commit --no-gpg-sign -m "pushed at %s"
git push fuseml master:main
`, tmpDir, u.String(), time.Now().Format("20060102150405")))

	output, err := cmd.CombinedOutput()
	if err != nil {
		c.ui.Problem().
			WithStringValue("Stdout", string(output)).
			WithStringValue("Stderr", "").
			Msg("App push failed")
		return errors.Wrap(err, "push script failed")
	}

	c.ui.Note().V(1).WithStringValue("Output", string(output)).Msg("")
	c.ui.Success().Msg("Application push successful")

	return nil
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

func (c *FusemlClient) waitForApp(org, name string) error {
	c.ui.ProgressNote().KeeplineUnder(1).Msg("Creating application resources")
	err := c.kubeClient.WaitUntilPodBySelectorExist(
		c.ui, c.config.FusemlWorkloadsNamespace,
		fmt.Sprintf("fuseml/app-guid=%s.%s", org, name),
		750)
	if err != nil {
		return errors.Wrap(err, "waiting for app to be created failed")
	}

	c.ui.ProgressNote().KeeplineUnder(1).Msg("Starting application")

	err = c.kubeClient.WaitForPodBySelectorRunning(
		c.ui, c.config.FusemlWorkloadsNamespace,
		fmt.Sprintf("fuseml/app-guid=%s.%s", org, name),
		300)

	if err != nil {
		return errors.Wrap(err, "waiting for app to come online failed")
	}

	return nil
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
	return strings.ReplaceAll(appDeployment.Items[0].Labels["fuseml/infer-url"], ".", "/"), nil
}
