package paas

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/fuseml/fuseml/cli/deployments"
	"github.com/fuseml/fuseml/cli/helpers"
	"github.com/fuseml/fuseml/cli/paas/ui"
	"github.com/fuseml/fuseml/cli/paas/version"
	"github.com/pkg/errors"
)

const (
	coreAPIURL            = "https://api.github.com/repos/fuseml/fuseml-core/releases"
	coreClientDownloadURL = "https://github.com/fuseml/fuseml-core/releases"
	coreClientName        = "fuseml"
)

// structure covering tiny part of github release json information
type Release struct {
	URL     string `json:"html_url,omitempty"`
	TagName string `json:"tag_name,omitempty"`
}

// Find out latest applicable version of the core client for the installer;
// installer version provided as argument
func latestClientForInstaller(version string) (string, error) {

	resp, err := http.Get(coreAPIURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, resp.Body)

	releases := []Release{}
	err = json.Unmarshal(buf.Bytes(), &releases)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse description file")
	}

	installer := strings.Split(version, ".")
	major := installer[0]
	minor := installer[1]

	clientRelease := ""

	// The list of releases is already sorted from the latest one,
	// so we do not need to check the patch version and just take the first one
	for _, release := range releases {
		tag := strings.Split(release.TagName, ".")
		coreMajor := tag[0]
		coreMinor := tag[1]
		if major == coreMajor && minor >= coreMinor {
			clientRelease = release.TagName
			break
		}
	}
	return clientRelease, nil
}

// download platform specific fuseml command line client to current directory
func downloadFuseMLCLI(ui *ui.UI, domain string) error {

	ui.Note().KeeplineUnder(1).Msg("Downloading command line client...")

	downloadURL := coreClientDownloadURL
	installerVersion := version.Version

	isRelease, err := regexp.MatchString(`^v[0-9]+\.[0-9]+\.?[0-9]*$`, installerVersion)
	if err != nil {
		return errors.Wrap(err, "failed to identify installer version")
	}

	if isRelease {
		clientVersion, err := latestClientForInstaller(installerVersion)
		if err != nil {
			return errors.Wrap(err, "failed to identify necessary client version")
		}
		if clientVersion == "" {
			return errors.New(fmt.Sprintf("Failed to find correct client version for installer version %s", installerVersion))
		}
		ui.Note().Msg(fmt.Sprintf("For installer version %s, downloading client version %s", installerVersion, clientVersion))
		downloadURL = downloadURL + "/download/" + clientVersion
	} else {
		downloadURL += "/latest/download"
		ui.Note().Msg("Downloading latest stable version of the client")
	}

	coreClientPlatform := fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
	name := fmt.Sprintf("%s-%s", coreClientName, coreClientPlatform)
	extension := "tar.gz"
	if runtime.GOOS == "windows" {
		extension = "zip"
	}

	url := fmt.Sprintf("%s/%s.%s", downloadURL, name, extension)
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

	ui.Success().Msg(fmt.Sprintf("FuseML command line client saved as %s.", path))
	ui.Normal().Msg("    It is recommended to copy it to the location within your PATH (e.g. /usr/local/bin).")

	fuseml_url := fmt.Sprintf("http://%s.%s", deployments.CoreDeploymentID, domain)
	ui.Note().Msg(fmt.Sprintf(
		"To use the FuseML CLI, you must point it to the FuseML server URL, e.g.:\n\n    export FUSEML_SERVER_URL=%s",
		fuseml_url))

	return nil
}
