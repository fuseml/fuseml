package deployments

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"

	"github.com/fuseml/fuseml/cli/helpers"
	"github.com/fuseml/fuseml/cli/kubernetes"
	"github.com/fuseml/fuseml/cli/paas/ui"
)

const (
	defaultDescriptionFileName = "description.yaml"
	tmpSubDir                  = "fuseml-extension"
)

type installInstruction struct {
	Type     string
	Location string
	Values   string
}

type extensionDesc struct {
	Name        string
	Description string
	Namespace   string
	Install     []installInstruction
	Uninstall   []installInstruction
}

type Extension struct {
	Name       string
	Repository string
	desc       *extensionDesc
}

func NewExtension(name, repository string) *Extension {
	return &Extension{
		Name:       name,
		Repository: repository,
		desc:       &extensionDesc{},
	}
}

// LoadDescription finds the description file of the extension and loads it into the struct
func (e Extension) LoadDescription() error {

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

	fmt.Printf("repo %s, loc %v\n", e.Repository, u)
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

func (e Extension) Install(c *kubernetes.Cluster, ui *ui.UI, options *kubernetes.InstallationOptions) error {

	// TODO based on installation type (script/helm/manifest), proceed with installation

	// TODO installation steps could have different namespaces...

	if e.desc.Namespace != "" {
		if err := c.LabelNamespace(e.desc.Namespace, kubernetes.FusemlDeploymentLabelKey, kubernetes.FusemlDeploymentLabelValue); err != nil {
			return err
		}
	}

	// TODO wait for some pod to exist/run? Extra option in the description file

	// TODO after installation, we might need to create istio ingress gateway! That means extra kubernetes manifest
	// (or maybe just boolean value indicating right functions are written from here)

	ui.Success().Msg(fmt.Sprintf("%s deployed.", e.Name))

	return nil
}
