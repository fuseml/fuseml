package deployments

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
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
	defaultNamespace           = "fuseml-workloads"
	tmpSubDir                  = "fuseml-extension"
)

type waitForStep struct {
	Kind      string
	Namespace string
	Condition string
	Selector  string
	Timeout   int
}

type installStep struct {
	Type      string
	Location  string
	Values    string
	Namespace string
	WaitFor   []waitForStep
}

type istioGateway struct {
	Namespace   string
	Name        string
	Port        int
	ServiceHost string
}

// extension structurs necessary to build a payload for extension registry
// (copied from fuseml-core/gen/extension/service.go for now...)
type registeredExtensionService struct {
	ID           *string
	ExtensionID  *string
	Resource     *string
	Category     *string
	Description  *string
	AuthRequired *bool
	Status       *registeredExtensionServiceStatus
	Endpoints    []*registeredExtensionEndpoint
	Credentials  []*registeredExtensionCredentials
}

// a query that can be run against the extension registry to retrieve
// (just a dummy structure for now)
type registeredExtensionQuery struct {
}

// extension information as expected by extension registry
type registeredExtension struct {
	ID            *string
	Product       *string
	Version       *string
	Description   *string
	Zone          *string
	Configuration map[string]string
	Status        *registeredExtensionStatus
	Services      []*registeredExtensionService
}

type registeredExtensionEndpoint struct {
	URL           *string
	ExtensionID   *string
	ServiceID     *string
	Type          *string
	Configuration map[string]string
	Status        *registeredExtensionEndpointStatus
}

type registeredExtensionCredentials struct {
	ID            *string
	ExtensionID   *string
	ServiceID     *string
	Default       *bool
	Scope         *string
	Projects      []string
	Users         []string
	Configuration map[string]string
	Status        *registeredExtensionCredentialsStatus
}

type registeredExtensionCredentialsStatus struct {
	Created string
	Updated string
}

type registeredExtensionStatus struct {
	Registered string
	Updated    string
}

type registeredExtensionServiceStatus struct {
	Registered string
	Updated    string
}

type registeredExtensionEndpointStatus struct {
}

// serviceCredentialTemplate describes the way how to generate service credentials
type serviceCredentialTemplate struct {
	ServiceID   string
	Credentials []credentialTemplate
}

type credentialTemplate struct {
	ID        string
	Transform []credentialTransformValue
}

type credentialTransformValue struct {
	ConfigValue string
	SecretValue string
	Secret      string
	Namespace   string
}

type extensionDesc struct {
	Name               string
	Product            string
	Version            string
	Description        string
	Namespace          string
	Zone               string
	Requires           []string
	Install            []installStep
	Uninstall          []installStep
	Gateways           []istioGateway
	Services           []registeredExtensionService
	ServiceCredentials []serviceCredentialTemplate
}

type Extension struct {
	Name                   string
	Repository             string
	Debug                  bool
	Timeout                int
	Desc                   *extensionDesc
	TransformedCredentials map[string]map[string]map[string]string
}

func NewExtension(name, repository string, timeout int) *Extension {
	return &Extension{
		Name:       name,
		Repository: repository,
		Desc:       &extensionDesc{},
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

	err = yaml.Unmarshal(data, &e.Desc)
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

// Pass the path string and return the absolute location of the directory
// If the path is relative, join it with the base repository path; if
// the path is URL, return the URL
func (e *Extension) getDirectoryPath(dirPath, tmpDir string) (string, error) {

	// 1, local path is absolute, return right away
	if filepath.IsAbs(dirPath) {
		return dirPath, nil
	}

	u, err := url.Parse(dirPath)
	if err != nil {
		return "", err
	}

	// 2. full URL, return as it is
	if u.IsAbs() && u.Host != "" {
		return dirPath, nil
	}
	// do not support relative path to main URL, the base URL is different for files and
	// kustomize directories...

	// 3. relative path to extension's local path
	return filepath.Join(e.Repository, e.Name, dirPath), nil
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

func (e *Extension) installManifest(ui *ui.UI, path, ns string) error {
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
	out, err := helpers.KubectlWithProgress(ui, kubectlCmd)

	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("kubectl apply failed:\n%s", out))
	}
	return nil
}

func (e *Extension) uninstallManifest(ui *ui.UI, path, ns string) error {
	tmpDir, err := ioutil.TempDir("", tmpSubDir)
	if err != nil {
		return errors.Wrap(err, "can't create temp directory "+tmpDir)
	}
	defer os.RemoveAll(tmpDir)

	manifestLocalPath, err := e.fetchFile(path, tmpDir)
	if err != nil {
		return errors.Wrap(err, "failed fetching file from "+path)
	}

	kubectlCmd := fmt.Sprintf("delete --filename %s --ignore-not-found", manifestLocalPath)
	if ns != "" {
		kubectlCmd = kubectlCmd + " --namespace " + ns
	}

	out, err := helpers.KubectlWithProgress(ui, kubectlCmd)

	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("kubectl delete failed:\n%s", out))
	}
	return nil
}

func (e *Extension) installKustomize(ui *ui.UI, path, ns string) error {
	tmpDir, err := ioutil.TempDir("", tmpSubDir)
	if err != nil {
		return errors.Wrap(err, "can't create temp directory "+tmpDir)
	}
	defer os.RemoveAll(tmpDir)

	kustomizeDir, err := e.getDirectoryPath(path, tmpDir)
	if err != nil {
		return errors.Wrap(err, "failed fetching directory from "+path)
	}

	kubectlCmd := fmt.Sprintf("apply --kustomize %s", kustomizeDir)
	if ns != "" {
		kubectlCmd = kubectlCmd + " --namespace " + ns
	}
	out, err := helpers.KubectlWithProgress(ui, kubectlCmd)

	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("kubectl apply failed:\n%s", out))
	}
	return nil
}

func (e *Extension) uninstallKustomize(ui *ui.UI, path, ns string) error {
	tmpDir, err := ioutil.TempDir("", tmpSubDir)
	if err != nil {
		return errors.Wrap(err, "can't create temp directory "+tmpDir)
	}
	defer os.RemoveAll(tmpDir)

	kustomizeDir, err := e.getDirectoryPath(path, tmpDir)
	if err != nil {
		return errors.Wrap(err, "failed fetching directory from "+path)
	}

	kubectlCmd := fmt.Sprintf("delete --kustomize %s --ignore-not-found", kustomizeDir)
	if ns != "" {
		kubectlCmd = kubectlCmd + " --namespace " + ns
	}
	out, err := helpers.KubectlWithProgress(ui, kubectlCmd)

	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("kubectl apply failed:\n%s", out))
	}
	return nil
}

// TODO move under helpers
func (e *Extension) installHelmChart(ui *ui.UI, name, chartPath, ns, valuesPath string) error {

	tmpDir, err := ioutil.TempDir("", tmpSubDir)
	if err != nil {
		return errors.Wrap(err, "can't create temp directory "+tmpDir)
	}
	defer os.RemoveAll(tmpDir)

	currentdir, err := os.Getwd()
	if err != nil {
		return err
	}

	helmCmd := fmt.Sprintf("helm list --namespace %s --deployed -q | grep %s", ns, name)
	if ns == "" {
		helmCmd = fmt.Sprintf("helm list --deployed -q | grep %s", name)
	}

	out, _ := helpers.RunProc(helmCmd, currentdir, e.Debug)
	if strings.TrimSpace(out) == name {
		ui.Exclamation().Msg(fmt.Sprintf("%s chart already present, skipping installation", name))
		return nil
	}

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

	helmCmd = fmt.Sprintf("helm install %s --create-namespace --values '%s' --namespace %s --wait %s", name, valuesLocalPath, ns, chartLocalPath)
	if ns == "" {
		helmCmd = fmt.Sprintf("helm install %s --values '%s' --wait %s", name, valuesLocalPath, chartLocalPath)
	}
	if out, err := helpers.RunProc(helmCmd, currentdir, e.Debug); err != nil {
		return errors.New(fmt.Sprintf("Failed installing %s chart (%s): %s", name, err, out))
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
	return c.LabelNamespace(coreDeploymentNamespace, kubernetes.FusemlDeploymentLabelKey, kubernetes.FusemlDeploymentLabelValue)
}

func (e *Extension) Uninstall(c *kubernetes.Cluster, ui *ui.UI, options *kubernetes.InstallationOptions) error {

	namespace := e.Desc.Namespace

	if namespace != "" {
		if notOurs, _ := c.NamespaceExistsAndNotOwned(namespace); notOurs == true {
			ui.Exclamation().Msg(fmt.Sprintf(
				"Namespace %s was not created by FuseML; not deleting extension %s",
				namespace, e.Name))
			return nil
		}
	}

	// based on installation type (script/helm/manifest), proceed with uninstallation of each install step
	for _, step := range e.Desc.Uninstall {

		ns := step.Namespace
		if ns == "" {
			ns = namespace
		} else {
			if notOurs, _ := c.NamespaceExistsAndNotOwned(ns); notOurs == true {
				ui.Exclamation().Msg(fmt.Sprintf(
					"Namespace exists but %s was not created by FuseML; skipping %s step of extension %s",
					ns, step.Type, e.Name))
				continue
			}
		}
		switch step.Type {
		case "helm":
			// TODO shoud step have a Name too? Could there be multiple helm charts?
			err := e.uninstallHelmChart(ui, e.Name, ns)
			if err != nil {
				return errors.Wrap(err, "failed to uninstall helm release "+e.Name)
			}
		case "manifest":
			err := e.uninstallManifest(ui, step.Location, ns)
			if err != nil {
				return errors.Wrap(err, "failed to uninstall kubernetes manifest from "+step.Location)
			}
		case "kustomize":
			err := e.uninstallKustomize(ui, step.Location, ns)
			if err != nil {
				return errors.Wrap(err, "failed to uninstall using kustomize directory "+step.Location)
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
		if step.Namespace != "" && step.Namespace != namespace && step.Namespace != defaultNamespace {
			if err := deleteNamespace(c, ui, step.Namespace); err != nil {
				return err
			}

		}
	}
	// delete namespace if it was specific to extension
	if e.Desc.Namespace != "" && e.Desc.Namespace != defaultNamespace {
		if err := deleteNamespace(c, ui, e.Desc.Namespace); err != nil {
			return err
		}
	}
	return nil
}

// Unregister extension from the extension registry
func (e *Extension) UnRegister(c *kubernetes.Cluster, ui *ui.UI, options *kubernetes.InstallationOptions) error {
	domain, err := options.GetString("system_domain", "")
	if err != nil {
		return errors.New("system_domain value not provided")
	}

	fusemlURL := fmt.Sprintf("http://%s.%s", CoreDeploymentID, domain)
	fullURL := fmt.Sprintf("%s/extensions/%s", fusemlURL, e.Desc.Name)

	// no direct Delete method so we have to create a client and a request
	client := &http.Client{}

	req, err := http.NewRequest("DELETE", fullURL, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode == 204 {
		return nil
	}

	// no such extension found, that might be OK...
	if resp.StatusCode == 404 {
		return nil
	}

	// something else is wrong, read the response from DELETE call
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, resp.Body)

	return errors.New(fmt.Sprintf("Failed unregistering the extension. Server returns %s: ", buf.String()))
}

// Read all extensions stored in extensions repository
func GetRegisteredExtensions(options *kubernetes.InstallationOptions) ([]registeredExtension, error) {

	extensions := make([]registeredExtension, 0)
	domain, err := options.GetString("system_domain", "")
	if err != nil {
		return extensions, errors.New("system_domain value not provided")
	}

	fusemlURL := fmt.Sprintf("http://%s.%s", CoreDeploymentID, domain)
	fullURL := fmt.Sprintf("%s/extensions", fusemlURL)

	query := registeredExtensionQuery{}

	// need a client as simle http.Get does not support adding a body
	client := &http.Client{}

	reqBody, err := json.Marshal(query)
	if err != nil {
		return extensions, err
	}

	req, err := http.NewRequest("GET", fullURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return extensions, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return extensions, err
	}

	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, resp.Body)

		err = yaml.Unmarshal(buf.Bytes(), &extensions)
		if err != nil {
			return extensions, errors.Wrap(err, "failed to parse description file")
		}
	} else {
		return extensions, errors.New(fmt.Sprintf("Unexpected response from registry: %d", resp.StatusCode))
	}

	return extensions, nil
}

// Check if an extension is already registered
// argument is the URL of the FuseML service
func (e *Extension) isExtensionRegistered(fusemlURL string) (bool, error) {

	fullURL := fmt.Sprintf("%s/extensions/%s", fusemlURL, e.Desc.Name)
	resp, err := http.Get(fullURL)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		return true, nil
	} else if resp.StatusCode == 404 {
		return false, nil
	}
	return false, errors.New(fmt.Sprintf("Unexpected response from registry: %d", resp.StatusCode))
}

// createTransformMaps goes through the 'servicecredentials' section in the description file
// and for each service/credential combination it fetches the right value from Kubernetes using
// the transformation rules written in said section
func (e *Extension) createTransformMaps(c *kubernetes.Cluster) error {
	// maps service id to map of credentials which maps credential id to value map, e.g.:
	// mlflow-store : { default-s3-account : { key1: value1, key2: value2 } }
	e.TransformedCredentials = make(map[string]map[string]map[string]string)
	for _, service := range e.Desc.ServiceCredentials {
		e.TransformedCredentials[service.ServiceID] = make(map[string]map[string]string)
		for _, cred := range service.Credentials {
			e.TransformedCredentials[service.ServiceID][cred.ID] = make(map[string]string)
			for _, transform := range cred.Transform {
				// now find the right value and save it to the map
				secret, err := c.GetSecret(transform.Namespace, transform.Secret)
				if err != nil {
					return err
				}
				e.TransformedCredentials[service.ServiceID][cred.ID][transform.ConfigValue] = string(secret.Data[transform.SecretValue])
			}
		}
	}
	return nil
}

// Register extension in the registry that is run by fuseml-core server
func (e *Extension) Register(c *kubernetes.Cluster, ui *ui.UI, options *kubernetes.InstallationOptions) error {

	domain, err := options.GetString("system_domain", "")
	if err != nil {
		return errors.New("system_domain value not provided")
	}

	fusemlURL := fmt.Sprintf("http://%s.%s", CoreDeploymentID, domain)

	registered, err := e.isExtensionRegistered(fusemlURL)
	if err != nil {
		return err
	}
	if registered {
		ui.Exclamation().Msg(fmt.Sprintf("Extension %s is already registered; if you want to update it, delete it first", e.Name))
		return nil
	}
	err = e.createTransformMaps(c)
	if err != nil {
		return errors.Wrap(err, "Failed to transform values for credentials")
	}

	extServices := []*registeredExtensionService{}
	for _, service := range e.Desc.Services {
		extServiceEndpoints := []*registeredExtensionEndpoint{}
		for _, endpoint := range service.Endpoints {
			serviceEndpoint := registeredExtensionEndpoint{
				URL:           endpoint.URL,
				Type:          endpoint.Type,
				Configuration: endpoint.Configuration,
			}
			extServiceEndpoints = append(extServiceEndpoints, &serviceEndpoint)
		}

		extServiceCredentials := []*registeredExtensionCredentials{}
		for _, creds := range service.Credentials {
			serviceCredentials := registeredExtensionCredentials{
				ID:            creds.ID,
				Default:       creds.Default,
				Scope:         creds.Scope,
				Projects:      creds.Projects,
				Users:         creds.Users,
				Configuration: make(map[string]string),
			}
			for key, val := range creds.Configuration {
				serviceCredentials.Configuration[key] = val
			}
			// update credential Configuration with the values from Transform section
			if e.TransformedCredentials[*service.ID][*creds.ID] != nil {
				for key, val := range e.TransformedCredentials[*service.ID][*creds.ID] {
					serviceCredentials.Configuration[key] = val
				}
			}
			extServiceCredentials = append(extServiceCredentials, &serviceCredentials)
		}

		extService := registeredExtensionService{
			ID:           service.ID,
			Resource:     service.Resource,
			Category:     service.Category,
			Description:  service.Description,
			AuthRequired: service.AuthRequired,
			Endpoints:    extServiceEndpoints,
			Credentials:  extServiceCredentials,
		}
		extServices = append(extServices, &extService)
	}

	ext := registeredExtension{
		ID:          &e.Name,
		Product:     &e.Desc.Product,
		Version:     &e.Desc.Version,
		Description: &e.Desc.Description,
		Services:    extServices,
	}

	jsonValue, err := json.Marshal(ext)
	if err != nil {
		return err
	}
	fullURL := fmt.Sprintf("%s/extensions", fusemlURL)

	resp, err := http.Post(fullURL, "application/json", bytes.NewBuffer(jsonValue))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 201 {
		return nil
	}

	// something wrong, read the response from POST call
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, resp.Body)

	return errors.New(fmt.Sprintf("Failed registering the extension. Server returns %s: ", buf.String()))
}

func (e *Extension) Install(c *kubernetes.Cluster, ui *ui.UI, options *kubernetes.InstallationOptions) error {

	namespace := e.Desc.Namespace
	if namespace != "" {
		// if namespace for an extension is already there (not created by FuseML), assume it is installed
		if notOurs, _ := c.NamespaceExistsAndNotOwned(namespace); notOurs == true {
			ui.Exclamation().Msg(fmt.Sprintf("Namespace %s is already present: assuming extension %s is already installed", namespace, e.Name))
			return nil
		}
		if err := createNamespace(c, namespace); err != nil {
			return err
		}
	}

	// based on installation type (script/helm/manifest), proceed with execution of each install step
	for _, step := range e.Desc.Install {
		ns := step.Namespace
		if ns != "" {
			if notOurs, _ := c.NamespaceExistsAndNotOwned(ns); notOurs == true {
				ui.Exclamation().Msg(fmt.Sprintf(
					"Namespace exists but %s was not created by FuseML; skipping %s step of extension %s",
					ns, step.Type, e.Name))
				continue
			}
			if err := createNamespace(c, ns); err != nil {
				return err
			}
		} else {
			// use the top namespace (it can still be empty though)
			ns = namespace
		}

		switch step.Type {
		case "helm":
			err := e.installHelmChart(ui, e.Name, step.Location, ns, step.Values)
			if err != nil {
				return errors.Wrap(err, "failed to install helm package from "+step.Location)
			}
		case "manifest":
			err := e.installManifest(ui, step.Location, ns)
			if err != nil {
				return errors.Wrap(err, "failed to install kubernetes manifest from "+step.Location)
			}
		case "kustomize":
			err := e.installKustomize(ui, step.Location, ns)
			if err != nil {
				return errors.Wrap(err, "failed to install from kustomize directory "+step.Location)
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
		if len(step.WaitFor) == 0 {
			continue
		}
		// wait until all wait steps are completed
		for _, waitStep := range step.WaitFor {

			condition := waitStep.Condition
			if condition == "" {
				condition = "Ready"
			}
			selection := "--all"
			if waitStep.Selector != "all" {
				selection = fmt.Sprintf("--selector=%s", waitStep.Selector)
			}
			timeout := waitStep.Timeout
			if timeout == 0 {
				timeout = e.Timeout
			}
			kind := waitStep.Kind
			if kind == "" {
				kind = "pod"
			}

			message := fmt.Sprintf("waiting for install step to finish waiting for %s", kind)
			out, err := helpers.WaitForCommandCompletion(ui, message,
				func() (string, error) {
					return helpers.Kubectl(fmt.Sprintf("wait --for=condition=%s %s --timeout=%ds -n %s %s",
						condition,
						selection,
						timeout,
						waitStep.Namespace,
						kind))
				},
			)
			if err != nil {
				return errors.Wrap(err, fmt.Sprintf("%s failed:\n%s", message, out))
			}
		}

	}

	if e.Desc.Namespace != "" {
		err := c.LabelNamespace(
			e.Desc.Namespace,
			kubernetes.FusemlDeploymentLabelKey,
			kubernetes.FusemlDeploymentLabelValue)
		if err != nil {
			return err
		}
	}

	// create istio gateways if required
	if c.HasIstio() && len(e.Desc.Gateways) > 0 {
		domain, err := options.GetString("system_domain", "")
		if err != nil {
			return errors.New("system_domain value not provided")
		}

		for _, g := range e.Desc.Gateways {

			ns := g.Namespace
			if ns == "" {
				ns = namespace
			}

			message := "Creating istio ingress gateway for " + g.Name
			subdomain := g.Name + "." + domain
			out, err := helpers.WaitForCommandCompletion(ui, message,
				func() (string, error) {
					return helpers.CreateIstioIngressGateway(g.Name, ns, subdomain, g.ServiceHost, g.Port)
				},
			)
			if err != nil {
				return errors.Wrap(err, fmt.Sprintf("%s failed:\n%s", message, out))
			}
			ui.Success().Msg(fmt.Sprintf("%s accessible at http://%s", g.Name, subdomain))
		}
	}

	ui.Success().Msg(fmt.Sprintf("%s deployed.", e.Name))

	return nil
}
