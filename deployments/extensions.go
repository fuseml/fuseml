package deployments

import (
	"context"
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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultDescriptionFileName = "description.yaml"
	tmpSubDir                  = "fuseml-extension"
)

type installStep struct {
	Type      string
	Location  string
	Values    string
	Namespace string
	WaitFor   string
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
	Timeout    int
	desc       *extensionDesc
}

func NewExtension(name, repository string, timeout int) *Extension {
	return &Extension{
		Name:       name,
		Repository: repository,
		desc:       &extensionDesc{},
		Debug:      false,
		Timeout:    timeout,
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

// Pass the path string and return the absolute location of the file
// If the path is relative, join it with the base repository path; if
// the path is URL download it and return path to downloaded copy
func (e *Extension) fetchFile(filePath, tmpDir string) (string, error) {

	// 1, local path is absolute, return right away
	if filepath.IsAbs(filePath) {
		return filePath, nil
	}

	name := filepath.Base(filePath)
	u, err := url.Parse(filePath)
	if err != nil {
		return "", err
	}
	// 2. full URL, download and return path to copy
	if u.IsAbs() && u.Host != "" {
		if err := helpers.DownloadFile(u.String(), name, tmpDir); err != nil {
			return "", err
		}
		return filepath.Join(tmpDir, name), nil
	}
	// 3. relative path to extension URL: adapt URL and download
	u, err = url.Parse(e.Repository)
	if u.IsAbs() && u.Host != "" {
		u, _ = u.Parse(e.Name + "/")
		u, _ = u.Parse(filePath)
		if err := helpers.DownloadFile(u.String(), name, tmpDir); err != nil {
			return "", err
		}
		return filepath.Join(tmpDir, name), nil
	}
	// 4. relative path to extension local path
	return filepath.Join(e.Repository, e.Name, filePath), nil
}

func (e *Extension) executeScript(path string) error {
	tmpDir, err := ioutil.TempDir("", tmpSubDir)
	if err != nil {
		return errors.Wrap(err, "can't create temp directory "+tmpDir)
	}
	defer os.RemoveAll(tmpDir)

	fullCmd, err := e.fetchFile(path, tmpDir)
	if err != nil {
		return errors.Wrap(err, "failed fetching file from "+path)
	}

	if err := os.Chmod(fullCmd, 0740); err != nil {
		return errors.New(fmt.Sprintf("Failed changing the file mode of %s", fullCmd))
	}

	if out, err := helpers.RunProc(fullCmd, tmpDir, e.Debug); err != nil {
		return errors.Wrap(err, fmt.Sprintf("Failed running script: %s\n", out))
	}

	return nil
}

func (e *Extension) installManifest(path, ns string) error {
	tmpDir, err := ioutil.TempDir("", tmpSubDir)
	if err != nil {
		return errors.Wrap(err, "can't create temp directory "+tmpDir)
	}
	defer os.RemoveAll(tmpDir)

	manifestLocalPath, err := e.fetchFile(path, tmpDir)
	if err != nil {
		return errors.Wrap(err, "failed fetching file from "+path)
	}

	kubectlCmd := fmt.Sprintf("apply --filename %s", manifestLocalPath)
	if ns != "" {
		kubectlCmd = kubectlCmd + " --namespace " + ns
	}
	out, err := helpers.Kubectl(kubectlCmd)

	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("kubectl apply failed:\n%s", out))
	}
	return nil
}

func (e *Extension) unInstallManifest(path, ns string) error {
	tmpDir, err := ioutil.TempDir("", tmpSubDir)
	if err != nil {
		return errors.Wrap(err, "can't create temp directory "+tmpDir)
	}
	defer os.RemoveAll(tmpDir)

	manifestLocalPath, err := e.fetchFile(path, tmpDir)
	if err != nil {
		return errors.Wrap(err, "failed fetching file from "+path)
	}

	kubectlCmd := fmt.Sprintf("delete --filename %s", manifestLocalPath)
	if ns != "" {
		kubectlCmd = kubectlCmd + " --namespace " + ns
	}

	out, err := helpers.Kubectl(kubectlCmd)

	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("kubectl delete failed:\n%s", out))
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
		valuesLocalPath, err = e.fetchFile(valuesPath, tmpDir)
		if err != nil {
			return errors.Wrap(err, "failed fetching file from "+valuesPath)
		}
	}

	helmCmd := fmt.Sprintf("helm install %s --create-namespace --values '%s' --namespace %s --wait %s", name, valuesLocalPath, ns, chartLocalPath)
	if ns == "" {
		helmCmd = fmt.Sprintf("helm install %s --values '%s' --wait %s", name, valuesLocalPath, chartLocalPath)
	}
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
			helmCmd := fmt.Sprintf("helm uninstall '%s'", name)
			if ns != "" {
				helmCmd = helmCmd + " --namespace " + ns
			}
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

func createNamespace(c *kubernetes.Cluster, ns string) error {
	if exists, _ := c.NamespaceExists(ns); exists == true {
		return nil
	}
	if _, err := c.Kubectl.CoreV1().Namespaces().Create(
		context.Background(),
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: ns,
			},
		},
		metav1.CreateOptions{},
	); err != nil {
		return err
	}
	return nil
}

func (e *Extension) Uninstall(c *kubernetes.Cluster, ui *ui.UI, options *kubernetes.InstallationOptions) error {

	namespace := e.desc.Namespace
	// based on installation type (script/helm/manifest), proceed with uninstallation of each install step
	for _, step := range e.desc.Uninstall {

		ns := step.Namespace
		if ns == "" {
			ns = namespace
		}
		switch step.Type {
		case "helm":
			// TODO shoud step have a Name too? Could there be multiple helm charts?
			err := e.uninstallHelmChart(ui, e.Name, ns)
			if err != nil {
				return errors.Wrap(err, "failed to uninstall helm release "+e.Name)
			}
		case "manifest":
			err := e.unInstallManifest(step.Location, ns)
			if err != nil {
				return errors.Wrap(err, "failed to uninstall kubernetes manifest from "+step.Location)
			}
		case "script":
			err := e.executeScript(step.Location)
			if err != nil {
				return errors.Wrap(err, "failed to install using "+step.Location)
			}
		default:
			return errors.New("Unsupported step type: " + step.Type)
		}
		// delete namespace if it was specific to step
		if step.Namespace != "" && step.Namespace != namespace {
			if err := deleteNamespace(c, ui, step.Namespace); err != nil {
				return err
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
	if namespace != "" {
		if err := createNamespace(c, namespace); err != nil {
			return err
		}
	}
	// based on installation type (script/helm/manifest), proceed with execution of each install step
	for _, step := range e.desc.Install {
		ns := step.Namespace
		if ns != namespace && ns != "" {
			if err := createNamespace(c, ns); err != nil {
				return err
			}
		}

		switch step.Type {
		case "helm":
			err := e.installHelmChart(e.Name, step.Location, ns, step.Values)
			if err != nil {
				return errors.Wrap(err, "failed to install helm package from "+step.Location)
			}
		case "manifest":
			err := e.installManifest(step.Location, ns)
			if err != nil {
				return errors.Wrap(err, "failed to install kubernetes manifest from "+step.Location)
			}
		case "script":
			err := e.executeScript(step.Location)
			if err != nil {
				return errors.Wrap(err, "failed to uninstall using "+step.Location)
			}
		default:
			return errors.New("Unsupported step type: " + step.Type)
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
		// wait until all pods in a namespace are running before proceeding with next step
		if step.WaitFor == "pods" {
			if err := c.WaitUntilPodBySelectorExist(ui, ns, "", e.Timeout); err != nil {
				return errors.Wrap(err, fmt.Sprintf("failed while waiting for pods in %s namespace to exist", ns))
			}
			if err := c.WaitForPodBySelectorRunning(ui, ns, "", e.Timeout); err != nil {
				return errors.Wrap(err, fmt.Sprintf("failed while waiting for pods in %s namespace to come up", ns))
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
