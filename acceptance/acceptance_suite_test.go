package acceptance_test

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/codeskyblue/kexec"
	"github.com/fuseml/fuseml/cli/helpers"
	"github.com/onsi/ginkgo/config"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestAcceptance(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Acceptance Suite")
}

var nodeSuffix, nodeTmpDir string
var failed = false

var serve string

func init() {
	flag.StringVar(&serve, "serve", "", "inference service to serve the model")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	if os.Getenv("REGISTRY_USERNAME") == "" || os.Getenv("REGISTRY_PASSWORD") == "" {
		fmt.Println("REGISTRY_USERNAME or REGISTRY_PASSWORD environment variables are empty. Pulling from dockerhub will be subject to rate limiting.")
	}

	if err := checkDependencies(); err != nil {
		panic("Missing dependencies: " + err.Error())
	}

	fmt.Printf("Compiling FuseML on node %d\n", config.GinkgoConfig.ParallelNode)

	buildFuseml()
	return []byte(strconv.Itoa(int(time.Now().Unix())))
}, func(randomSuffix []byte) {
	var err error

	nodeSuffix = fmt.Sprintf("%d-%s",
		config.GinkgoConfig.ParallelNode, string(randomSuffix))
	nodeTmpDir, err = ioutil.TempDir("", "fuseml-"+nodeSuffix)
	if err != nil {
		panic("Could not create temp dir: " + err.Error())
	}

	copyFuseml()
	// NOTE: Don't set FUSEML_ACCEPTANCE_KUBECONFIG when using multiple ginkgo
	// nodes because they will all use the same cluster. This will lead to flaky
	// tests.
	if kubeconfigPath := os.Getenv("FUSEML_ACCEPTANCE_KUBECONFIG"); kubeconfigPath != "" {
		os.Setenv("KUBECONFIG", kubeconfigPath)
	} else {
		fmt.Printf("Creating a cluster for node %d\n", config.GinkgoConfig.ParallelNode)
		createCluster()
		os.Setenv("KUBECONFIG", nodeTmpDir+"/kubeconfig")
	}
	os.Setenv("FUSEML_CONFIG", nodeTmpDir+"/fuseml.yaml")

	if os.Getenv("REGISTRY_USERNAME") != "" && os.Getenv("REGISTRY_PASSWORD") != "" {
		fmt.Printf("Creating image pull secret for Dockerhub on node %d\n", config.GinkgoConfig.ParallelNode)
		helpers.Kubectl(fmt.Sprintf("create secret docker-registry regcred --docker-server=%s --docker-username=%s --docker-password=%s",
			"https://index.docker.io/v1/",
			os.Getenv("REGISTRY_USERNAME"),
			os.Getenv("REGISTRY_PASSWORD"),
		))
	}

	fmt.Printf("Installing FuseML on node %d\n", config.GinkgoConfig.ParallelNode)
	installFuseml()

	if serve == "knative" {
		fmt.Printf("Installing Knative on node %d\n", config.GinkgoConfig.ParallelNode)
		installKnative()
	}

	if serve == "kfserving" {
		fmt.Printf("Installing KFServing on node %d\n", config.GinkgoConfig.ParallelNode)
		installKfserving()
	}

	if strings.Contains(serve, "seldon") {
		fmt.Printf("Installing seldon on node %d\n", config.GinkgoConfig.ParallelNode)
		installSeldon()
	}

	upgradeFuseml()
})

var _ = AfterEach(func() {
	failed = failed || CurrentGinkgoTestDescription().Failed
})

var _ = AfterSuite(func() {
	if failed {
		printClusterInfo()
	}
	fmt.Printf("Uninstall fuseml on node %d\n", config.GinkgoConfig.ParallelNode)
	out, _ := uninstallFuseml()
	match, _ := regexp.MatchString(`FuseML uninstalled`, out)
	if !match {
		panic("Uninstalling fuseml failed: " + out)
	}

	if os.Getenv("FUSEML_ACCEPTANCE_KUBECONFIG") == "" {
		fmt.Printf("Deleting cluster on node %d\n", config.GinkgoConfig.ParallelNode)
		deleteCluster()
	}

	fmt.Printf("Deleting tmpdir on node %d\n", config.GinkgoConfig.ParallelNode)
	deleteTmpDir()

})

func createCluster() {
	name := fmt.Sprintf("fuseml-acceptance-%s", nodeSuffix)

	if _, err := exec.LookPath("k3d"); err != nil {
		panic("Couldn't find k3d in PATH: " + err.Error())
	}

	_, err := RunProc("k3d cluster create --agents 1 --k3s-server-arg '--no-deploy=traefik' --k3s-server-arg '--kubelet-arg=eviction-hard=imagefs.available<1%,nodefs.available<1%' --k3s-server-arg '--kubelet-arg=eviction-minimum-reclaim=imagefs.available=1%,nodefs.available=1%' --k3s-agent-arg '--kubelet-arg=eviction-hard=imagefs.available<1%,nodefs.available<1%' --k3s-agent-arg '--kubelet-arg=eviction-minimum-reclaim=imagefs.available=1%,nodefs.available=1%' "+name, nodeTmpDir, false)
	if err != nil {
		panic("Creating k3d cluster failed: " + err.Error())
	}

	kubeconfig, err := RunProc("k3d kubeconfig get "+name, nodeTmpDir, false)
	if err != nil {
		panic("Getting kubeconfig failed: " + err.Error())
	}
	err = ioutil.WriteFile(path.Join(nodeTmpDir, "kubeconfig"), []byte(kubeconfig), 0644)
	if err != nil {
		panic("Writing kubeconfig failed: " + err.Error())
	}
}

func deleteCluster() {
	name := fmt.Sprintf("fuseml-acceptance-%s", nodeSuffix)

	if _, err := exec.LookPath("k3d"); err != nil {
		panic("Couldn't find k3d in PATH: " + err.Error())
	}

	output, err := RunProc("k3d cluster delete "+name, nodeTmpDir, false)
	if err != nil {
		panic(fmt.Sprintf("Deleting k3d cluster failed: %s\n %s\n",
			output, err.Error()))
	}
}

func deleteTmpDir() {
	err := os.RemoveAll(nodeTmpDir)
	if err != nil {
		panic(fmt.Sprintf("Failed deleting temp dir %s: %s\n",
			nodeTmpDir, err.Error()))
	}
}

func installKnative() {
	_, err := RunProc("make knative-install", "..", true)
	if err != nil {
		panic("Installing Knative failed: " + err.Error())
	}
}

func installKfserving() {
	_, err := RunProc("make kfserving-install", "..", true)
	if err != nil {
		panic("Installing KFServing failed: " + err.Error())
	}
}

func installSeldon() {
	_, err := RunProc("make seldon-install", "..", true)
	if err != nil {
		panic("Installing Seldon Operator failed: " + err.Error())
	}
}

func printClusterInfo() {
	_, err := RunProc("df -h; kubectl get pods -A; kubectl top nodes; kubectl top pods -A; kubectl describe nodes; kubectl describe pods -A", "..", true)
	if err != nil {
		panic("Getting kubernetes info failed: " + err.Error())
	}
}

func RunProc(cmd, dir string, toStdout bool) (string, error) {
	p := kexec.CommandString(cmd)

	var b bytes.Buffer
	if toStdout {
		p.Stdout = io.MultiWriter(os.Stdout, &b)
		p.Stderr = io.MultiWriter(os.Stderr, &b)
	} else {
		p.Stdout = &b
		p.Stderr = &b
	}

	p.Dir = dir

	if err := p.Run(); err != nil {
		return b.String(), err
	}

	err := p.Wait()
	return b.String(), err
}

func buildFuseml() {
	output, err := RunProc("make", "..", false)
	if err != nil {
		panic(fmt.Sprintf("Couldn't build FuseML: %s\n %s\n"+err.Error(), output))
	}
}

func copyFuseml() {
	output, err := RunProc("cp dist/fuseml-installer "+nodeTmpDir+"/", "..", false)
	if err != nil {
		panic(fmt.Sprintf("Couldn't copy FuseML: %s\n %s\n"+err.Error(), output))
	}
}

func installFuseml() (string, error) {
	return Fuseml("install", "")
}

func upgradeFuseml() (string, error) {
	return Fuseml("upgrade", "")
}

func uninstallFuseml() (string, error) {
	return Fuseml("uninstall", "")
}

// Fuseml invokes the `fuseml` binary, running the specified command.
// It returns the command output and/or error.
// dir parameter defines the directory from which the command should be run.
// It defaults to the current dir if left empty.
func Fuseml(command string, dir string) (string, error) {
	var commandDir string
	var err error

	if dir == "" {
		commandDir, err = os.Getwd()
		if err != nil {
			return "", err
		}
	} else {
		commandDir = dir
	}

	cmd := fmt.Sprintf(nodeTmpDir+"/fuseml-installer --verbosity 1 %s", command)

	return RunProc(cmd, commandDir, true)
}

func checkDependencies() error {
	ok := true

	dependencies := []struct {
		CommandName string
	}{
		{CommandName: "wget"},
		{CommandName: "tar"},
	}

	for _, dependency := range dependencies {
		_, err := exec.LookPath(dependency.CommandName)
		if err != nil {
			fmt.Printf("Not found: %s\n", dependency.CommandName)
			ok = false
		}
	}

	if ok {
		return nil
	}

	return errors.New("Please check your PATH, some of our dependencies were not found")
}
