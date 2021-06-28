package deployments

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"

	"github.com/fuseml/fuseml/cli/helpers"
	"github.com/fuseml/fuseml/cli/kubernetes"
	"github.com/fuseml/fuseml/cli/paas/ui"
)

const (
	defaultDescriptionFileName = "description.yaml"
	defaultNamespace           = "fuseml-workloads"
	tmpSubDir                  = "fuseml-extension"
)

type installStep struct {
	Type      string
	Location  string
	Values    string
	Namespace string
}

type istioGateway struct {
	Name        string
	Port        int
	ServiceHost string
}

type extensionDesc struct {
	Name        string
	Description string
	Namespace   string
	Install     []installStep
	Uninstall   []installStep
	Gateways    []istioGateway
}

type Extension struct {
	Name       string
	Repository string
	Debug      bool
	desc       *extensionDesc
}

func NewExtension(name, repository string) *Extension {
	return &Extension{
		Name:       name,
		Repository: repository,
		desc:       &extensionDesc{},
		Debug:      false,
	}
}

// LoadDescription finds the description file of the extension and loads it into the struct
func (e *Extension) LoadDescription() error {

	u, err := url.Parse(e.Repository)
	if err != nil {
		return err
	}

	tmpDir, err := ioutil.TempDir("", tmpSubDir)
	if err != nil {
		return errors.Wrap(err, "can't create temp directory "+tmpDir)
	}
	defer os.RemoveAll(tmpDir)

	descFilePath := ""

	if u.IsAbs() && u.Scheme != "" && u.Host != "" {
		// "/" at the end is necessary so that last part of the path is not replaced
		u, _ = u.Parse(e.Name + "/")
		u, _ = u.Parse(defaultDescriptionFileName)
		if err := helpers.DownloadFile(u.String(), defaultDescriptionFileName, tmpDir); err != nil {
			return err
		}
		descFilePath = filepath.Join(tmpDir, defaultDescriptionFileName)
	} else {
		info, err := os.Stat(e.Repository)
		if os.IsNotExist(err) {
			return err
		}
		if !info.IsDir() {
			return errors.New("Provided path to extension repository is neither URL nor a directory")

		}
		descFilePath = filepath.Join(e.Repository, e.Name, defaultDescriptionFileName)
	}

	// parse and load descriptin file into Extension struct
	data, err := os.ReadFile(descFilePath)
	if err != nil {
		return errors.Wrap(err, "failed to read description file")
	}

	err = yaml.Unmarshal(data, &e.desc)
	if err != nil {
		return errors.Wrap(err, "failed to parse description file")
	}
	return nil
}

// TODO move under helpers
func (e *Extension) installHelmChart(name, chartPath, ns, valuesPath string) error {

	tmpDir, err := ioutil.TempDir("", tmpSubDir)
	if err != nil {
		return errors.Wrap(err, "can't create temp directory "+tmpDir)
	}
	defer os.RemoveAll(tmpDir)

	tarName := filepath.Base(chartPath)
	if err = helpers.DownloadFile(chartPath, tarName, tmpDir); err != nil {
		return errors.Wrap(err, "can't download helm chart for "+name)
	}

	chartLocalPath := filepath.Join(tmpDir, tarName)
	valuesLocalPath := ""

	if valuesPath != "" {
		// FIXME valuesPath might be relative!
		if err = helpers.DownloadFile(valuesPath, "values.yaml", tmpDir); err != nil {
			return errors.Wrap(err, "can't download values.yaml for "+name)
		}
		valuesLocalPath = filepath.Join(tmpDir, "values.yaml")
	}

	helmCmd := fmt.Sprintf("helm install %s --create-namespace --values '%s' --namespace %s --wait %s", name, valuesLocalPath, ns, chartLocalPath)
	currentdir, err := os.Getwd()
	if err != nil {
		return err
	}
	if out, err := helpers.RunProc(helmCmd, currentdir, e.Debug); err != nil {
		return errors.New(fmt.Sprintf("Failed installing %s chart: %s", name, out))
	}

	return nil
}

func (e *Extension) uninstallHelmChart(ui *ui.UI, name, ns string) error {

	currentdir, err := os.Getwd()
	if err != nil {
		return err
	}
	out, err := helpers.WaitForCommandCompletion(ui, "Removing helm release "+name,
		func() (string, error) {
			helmCmd := fmt.Sprintf("helm uninstall '%s' --namespace '%s'", name, ns)
			return helpers.RunProc(helmCmd, currentdir, e.Debug)
		},
	)
	if err != nil {
		if strings.Contains(out, "release: not found") {
			ui.Exclamation().Msgf("%s helm release not found, skipping.\n", name)
		} else {
			return errors.Wrapf(err, "Failed uninstalling helm release %s: %s", name, out)
		}
	}

	return nil
}

func deleteNamespace(c *kubernetes.Cluster, ui *ui.UI, ns string) error {

	_, err := helpers.WaitForCommandCompletion(ui, "Deleting namespace "+ns,
		func() (string, error) {
			return "", c.DeleteNamespace(ns)
		},
	)
	if err != nil {
		return errors.Wrapf(err, "Failed deleting namespace %s", ns)
	}
	return nil
}

func (e *Extension) Uninstall(c *kubernetes.Cluster, ui *ui.UI, options *kubernetes.InstallationOptions) error {

	namespace := e.desc.Namespace
	if namespace == "" {
		namespace = defaultNamespace
	}
	// based on installation type (script/helm/manifest), proceed with uninstallation of each install step
	for _, step := range e.desc.Uninstall {

		switch step.Type {
		case "helm":
			ns := step.Namespace
			if ns == "" {
				ns = namespace
			}
			// TODO shoud step have a Name too? Could there be multiple helm charts?
			err := e.uninstallHelmChart(ui, e.Name, ns)
			if err != nil {
				return errors.Wrap(err, "failed to uninstall helm release "+e.Name)
			}

			// delete namespace if it was specific to step
			if step.Namespace != "" && step.Namespace != namespace {
				if err := deleteNamespace(c, ui, step.Namespace); err != nil {
					return err
				}

			}
		}
	}
	// delete namespace if it was specific to extension
	if e.desc.Namespace != "" {
		if err := deleteNamespace(c, ui, e.desc.Namespace); err != nil {
			return err
		}
	}
	return nil
}

func (e *Extension) Install(c *kubernetes.Cluster, ui *ui.UI, options *kubernetes.InstallationOptions) error {

	namespace := e.desc.Namespace
	if namespace == "" {
		namespace = defaultNamespace
	}
	// based on installation type (script/helm/manifest), proceed with installation of each install step
	for _, step := range e.desc.Install {

		switch step.Type {
		case "helm":
			ns := step.Namespace
			if ns == "" {
				ns = namespace
			}
			err := e.installHelmChart(e.Name, step.Location, ns, step.Values)
			if err != nil {
				return errors.Wrap(err, "failed to install helm package from "+step.Location)
			}

			if step.Namespace != "" && step.Namespace != namespace {
				err := c.LabelNamespace(
					step.Namespace,
					kubernetes.FusemlDeploymentLabelKey,
					kubernetes.FusemlDeploymentLabelValue)
				if err != nil {
					return err
				}
			}
		}
	}

	if e.desc.Namespace != "" {
		err := c.LabelNamespace(
			e.desc.Namespace,
			kubernetes.FusemlDeploymentLabelKey,
			kubernetes.FusemlDeploymentLabelValue)
		if err != nil {
			return err
		}
	}

	// TODO wait for some pod to exist/run? Extra option in the description file

	// create istio gateways if required
	if c.HasIstio() && len(e.desc.Gateways) > 0 {
		domain, err := options.GetString("system_domain", "")
		if err != nil {
			return errors.New("system_domain value not provided")
		}

		for _, g := range e.desc.Gateways {

			message := "Creating istio ingress gateway for " + g.Name
			subdomain := g.Name + "." + domain
			out, err := helpers.WaitForCommandCompletion(ui, message,
				func() (string, error) {
					return helpers.CreateIstioIngressGateway(g.Name, namespace, subdomain, g.ServiceHost, g.Port)
				},
			)
			if err != nil {
				return errors.Wrap(err, fmt.Sprintf("%s failed:\n%s", message, out))
			}

		}
	}

	ui.Success().Msg(fmt.Sprintf("%s deployed.", e.Name))

	return nil
}
