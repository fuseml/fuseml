package helpers

import (
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"

	"github.com/pkg/errors"
)

// DownloadFile downloads the given url as a file with "name" under "directory"
func DownloadFile(url, name, directory string) error {

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		return errors.New(url + " not found")
	}
	defer resp.Body.Close()

	out, err := os.Create(path.Join(directory, name))
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// DownloadAndUntar downloads the given url and decopresses it under the target directory
func DownloadAndUntar(url, targetDir string) error {

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return UntarStream(resp.Body, targetDir)
}

// DownloadAndUnzip downloads the archive from given url and decopresses it under the target directory
func DownloadAndUnzip(url, targetDir string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	tmpDir, err := ioutil.TempDir("", "fuseml-core")
	if err != nil {
		return errors.Wrap(err, "can't create temp directory for download")
	}
	defer os.Remove(tmpDir)

	tmpPath := path.Join(tmpDir, "archive.zip")
	out, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)

	return Unzip(tmpPath, targetDir)
}
