package paas

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/fuseml/fuseml/cli/deployments"
	"github.com/fuseml/fuseml/cli/helpers"
	"github.com/fuseml/fuseml/cli/paas/ui"
	"github.com/pkg/errors"
)

const (
	coreClientDownloadURL = "https://github.com/fuseml/fuseml-core/releases/latest/download"
	coreClientName        = "fuseml"
)

// download platform specific fuseml command line client to current directory
func downloadFuseMLCLI(ui *ui.UI, domain string) error {

	ui.Note().KeeplineUnder(1).Msg("Downloading command line client...")

	coreClientPlatform := fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
	name := fmt.Sprintf("%s-%s", coreClientName, coreClientPlatform)
	extension := "tar.gz"
	if runtime.GOOS == "windows" {
		extension = "zip"
	}

	url := fmt.Sprintf("%s/%s.%s", coreClientDownloadURL, name, extension)
	dir, err := os.Getwd()
	if err != nil {
		return errors.New("Failed geting current directory")
	}
	path := filepath.Join(dir, coreClientName)

	if runtime.GOOS == "windows" {
		err = helpers.DownloadAndUnzip(url, dir)
	} else {
		err = helpers.DownloadAndUntar(url, dir)
	}
	if err != nil {
		return errors.New(fmt.Sprintf("Failed downloading client from %s: %s", url, err.Error()))
	}

	ui.Note().Msg(fmt.Sprintf(
		"FuseML command line client saved as %s.\nCopy it to the location within your PATH (e.g. /usr/local/bin).",
		path))

	fuseml_url := fmt.Sprintf("http://%s.%s", deployments.CoreDeploymentID, domain)
	ui.Note().Msg(fmt.Sprintf(
		"To use the FuseML CLI, you must point it to the FuseML server URL, e.g.:\n\n    export FUSEML_SERVER_URL=%s",
		fuseml_url))

	return nil
}
