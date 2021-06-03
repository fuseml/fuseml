package paas

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/fuseml/fuseml/cli/helpers"
	"github.com/fuseml/fuseml/cli/paas/ui"
	"github.com/pkg/errors"
)

const (
        coreClientDownloadURL = "https://github.com/fuseml/fuseml-core/releases/download/v0.1.0"
	coreClientName        = "fuseml"
)

// download platform specific fuseml command line client to current directory
func downloadFuseMLCLI(ui *ui.UI) error {

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
	return nil
}
