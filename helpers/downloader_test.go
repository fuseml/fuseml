package helpers_test

import (
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"

	. "github.com/fuseml/fuseml/cli/helpers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("DownloadFile", func() {
	var url, directory string
	var err error

	BeforeEach(func() {
		directory, err = ioutil.TempDir("", "fuseml-test")

		file, err := os.Create(path.Join(directory, "thefile"))
		Expect(err).ToNot(HaveOccurred())
		defer file.Close()

		file.Write([]byte("the file contents"))

		dirURL, err := ServeDir(directory)
		Expect(err).ToNot(HaveOccurred())

		url = "http://" + dirURL + "/thefile"
	})
	AfterEach(func() {
		os.RemoveAll(directory)
	})

	It("downloads a url with filename under directory", func() {
		err = DownloadFile(url, "downloadedFile", directory)
		Expect(err).ToNot(HaveOccurred())
		targetPath := path.Join(directory, "downloadedFile")

		Expect(targetPath).To(BeARegularFile())

		contents, err := ioutil.ReadFile(targetPath)
		Expect(err).ToNot(HaveOccurred())
		Expect(string(contents)).To(Equal("the file contents"))
	})
})

// ServeDir serves the directory on a random port over http and returns the url
// where it can be reached.
func ServeDir(directory string) (string, error) {
	http.Handle("/", http.FileServer(http.Dir(directory)))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}

	go func() {
		panic(http.Serve(listener, nil))
	}()

	return listener.Addr().String(), nil
}
