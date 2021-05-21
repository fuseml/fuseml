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
	coreClientDownloadURL = "https://github.com/fuseml/fuseml-core/releases/latest/download"
	coreClientName        = "fuseml"
)

// download platform specific fuseml command line client to current directory
func downloadFuseMLCLI(ui *ui.UI) error {

	ui.Note().KeeplineUnder(1).Msg("Downloading command line client...")

	coreClientPlatform := fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
	name := fmt.Sprintf("%s-%s", coreClientName, coreClientPlatform)
	url := fmt.Sprintf("%s/%s.tar.gz", coreClientDownloadURL, name)
	dir, err := os.Getwd()
	if err != nil {
		return errors.New("Failed geting current directory")
	}
	path := filepath.Join(dir, coreClientName)

	if err := helpers.DownloadAndUntar(url, dir); err != nil {
		return errors.New(fmt.Sprintf("Failed downloading client from %s: %s", url, err.Error()))
	}

	ui.Note().Msg(fmt.Sprintf(
		"FuseML command line client saved as %s.\nCopy it to the location within your PATH (e.g. /usr/local/bin).",
		path))
	return nil
}
