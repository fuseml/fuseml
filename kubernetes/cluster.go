package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/pkg/errors"

	generic "github.com/fuseml/fuseml/cli/kubernetes/platform/generic"
	ibm "github.com/fuseml/fuseml/cli/kubernetes/platform/ibm"
	k3s "github.com/fuseml/fuseml/cli/kubernetes/platform/k3s"
	kind "github.com/fuseml/fuseml/cli/kubernetes/platform/kind"
	minikube "github.com/fuseml/fuseml/cli/kubernetes/platform/minikube"
	"github.com/fuseml/fuseml/cli/paas/ui"

	v1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"

	// https://github.com/kubernetes/client-go/issues/345
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

const (
	// APISGroupName is the api name used for fuseml
	APISGroupName = "fuse.ml"
)

var (
	FusemlDeploymentLabelKey   = fmt.Sprintf("%s/%s", APISGroupName, "deployment")
	FusemlDeploymentLabelValue = "true"
)

type Platform interface {
	Detect(*kubernetes.Clientset) bool
	Describe() string
	String() string
	Load(*kubernetes.Clientset) error
	ExternalIPs() []string
}

var SupportedPlatforms []Platform = []Platform{
	kind.NewPlatform(),
	k3s.NewPlatform(),
	ibm.NewPlatform(),
	minikube.NewPlatform(),
}

type Cluster struct {
	//	InternalIPs []string
	//	Ingress     bool
	Kubectl    *kubernetes.Clientset
	RestConfig *restclient.Config
	platform   Platform
}

// NewClusterFromClient creates a new Cluster from a Kubernetes rest client config
func NewClusterFromClient(restConfig *restclient.Config) (*Cluster, error) {
	c := &Cluster{}

	c.RestConfig = restConfig
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}
	c.Kubectl = clientset
	c.detectPlatform()
	if c.platform == nil {
		c.platform = generic.NewPlatform()
	}

	return c, c.platform.Load(clientset)
}

func NewCluster(kubeconfig string) (*Cluster, error) {
	c := &Cluster{}
	return c, c.Connect(kubeconfig)
}

func (c *Cluster) GetPlatform() Platform {
	return c.platform
}

func (c *Cluster) Connect(config string) error {
	restConfig, err := clientcmd.BuildConfigFromFlags("", config)
	if err != nil {
		return err
	}
	c.RestConfig = restConfig
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return err
	}
	c.Kubectl = clientset
	c.detectPlatform()
	if c.platform == nil {
		c.platform = generic.NewPlatform()
	}

	err = c.platform.Load(clientset)
	if err == nil {
		fmt.Println(c.platform.Describe())
	}
	return err
}

func (c *Cluster) detectPlatform() {
	for _, p := range SupportedPlatforms {
		if p.Detect(c.Kubectl) {
			c.platform = p
			return
		}
	}
}

// IsPodRunningAndReady returns a condition function that indicates whether the given pod is
// currently running and ready
func (c *Cluster) IsPodRunningAndReady(podName, namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		pod, err := c.Kubectl.CoreV1().Pods(namespace).Get(context.Background(), podName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		for _, cont := range pod.Status.ContainerStatuses {
			if cont.State.Waiting != nil {
				//fmt.Println("containers still in waiting")
				return false, nil
			}
		}

		for _, cont := range pod.Status.InitContainerStatuses {
			if cont.State.Waiting != nil || cont.State.Running != nil {
				return false, nil
			}
		}

		for _, cont := range pod.Status.ContainerStatuses {
			if cont.Ready != true {
				return false, nil
			}
		}

		switch pod.Status.Phase {
		case v1.PodRunning, v1.PodSucceeded:
			return true, nil
		case v1.PodFailed:
			return false, nil
		}
		return false, nil
	}
}

func (c *Cluster) PodExists(namespace, selector string) wait.ConditionFunc {
	return func() (bool, error) {
		podList, err := c.ListPods(namespace, selector)
		if err != nil {
			return false, err
		}
		if len(podList.Items) == 0 {
			return false, nil
		}
		return true, nil
	}
}

// Poll up to timeout seconds for pod to enter running and ready state.
// Returns an error if the pod never enters such state.
func (c *Cluster) WaitForPodRunning(namespace, podName string, timeout time.Duration) error {
	return wait.PollImmediate(time.Second, timeout, c.IsPodRunningAndReady(podName, namespace))
}

// ListPods returns the list of currently scheduled or running pods in `namespace` with the given selector
func (c *Cluster) ListPods(namespace, selector string) (*v1.PodList, error) {
	listOptions := metav1.ListOptions{}
	if len(selector) > 0 {
		listOptions.LabelSelector = selector
	}
	podList, err := c.Kubectl.CoreV1().Pods(namespace).List(context.Background(), listOptions)
	if err != nil {
		return nil, err
	}
	return podList, nil
}

// Wait up to timeout seconds for all pods in 'namespace' with given 'selector' to enter running state.
// Returns an error if no pods are found or not all discovered pods enter running state.
func (c *Cluster) WaitUntilPodBySelectorExist(ui *ui.UI, namespace, selector string, timeout int) error {
	s := ui.Progressf("Creating %s in %s", selector, namespace)
	defer s.Stop()

	return wait.PollImmediate(time.Second, time.Duration(timeout)*time.Second, c.PodExists(namespace, selector))
}

// WaitForPodBySelectorRunning waits timeout seconds for all pods in 'namespace'
// with given 'selector' to enter running state. Returns an error if no pods are
// found or not all discovered pods enter running state.
func (c *Cluster) WaitForPodBySelectorRunning(ui *ui.UI, namespace, selector string, timeout int) error {
	s := ui.Progressf("Starting %s in %s", selector, namespace)
	defer s.Stop()

	podList, err := c.ListPods(namespace, selector)
	if err != nil {
		return errors.Wrapf(err, "failed listingpods with selector %s", selector)
	}

	if len(podList.Items) == 0 {
		return fmt.Errorf("no pods in %s with selector %s", namespace, selector)
	}

	for _, pod := range podList.Items {
		s.ChangeMessagef("  Starting pod %s in %s", pod.Name, namespace)
		if err := c.WaitForPodRunning(namespace, pod.Name, time.Duration(timeout)*time.Second); err != nil {
			events, err2 := c.GetPodEvents(namespace, pod.Name)
			if err2 != nil {
				return errors.Wrap(err, err2.Error())
			} else {
				return errors.New(fmt.Sprintf("Failed waiting for %s: %s\nPod Events: \n%s", pod.Name, err.Error(), events))
			}
		}
	}
	return nil
}

// GetPodEventsWithSelector tries to find a pod using the provided selector and
// namespace. If found it returns the events on that Pod. If not found it returns
// an error.
// An equivalent kubectl command would look like this
// (label selector being "app.kubernetes.io/name=container-registry"):
//   kubectl get event --namespace my-namespace \
//   --field-selector involvedObject.name=$( \
//     kubectl get pods -o=jsonpath='{.items[0].metadata.name}' --selector=app.kubernetes.io/name=container-registry -n my-namespace)
func (c *Cluster) GetPodEventsWithSelector(namespace, selector string) (string, error) {
	podList, err := c.Kubectl.CoreV1().Pods(namespace).List(context.Background(),
		metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", err
	}
	if len(podList.Items) < 1 {
		return "", errors.New(fmt.Sprintf("Couldn't find Pod with selector '%s' in namespace %s", selector, namespace))
	}
	podName := podList.Items[0].Name

	return c.GetPodEvents(namespace, podName)
}

func (c *Cluster) GetPodEvents(namespace, podName string) (string, error) {
	eventList, err := c.Kubectl.CoreV1().Events(namespace).List(context.Background(),
		metav1.ListOptions{
			FieldSelector: "involvedObject.name=" + podName,
		})
	if err != nil {
		return "", err
	}

	events := []string{}
	for _, event := range eventList.Items {
		events = append(events, event.Message)
	}

	return strings.Join(events, "\n"), nil
}

func (c *Cluster) Exec(namespace, podName, containerName string, command, stdin string) (string, string, error) {
	var stdout, stderr bytes.Buffer
	stdinput := bytes.NewBuffer([]byte(stdin))

	err := c.execPod(namespace, podName, containerName, command, stdinput, &stdout, &stderr)

	// if options.PreserveWhitespace {
	// 	return stdout.String(), stderr.String(), err
	// }
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

// GetSecret gets a secret's values
func (c *Cluster) GetSecret(namespace, name string) (*v1.Secret, error) {
	secret, err := c.Kubectl.CoreV1().Secrets(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get secret")
	}

	return secret, nil
}

// GetVersion get the kube server version
func (c *Cluster) GetVersion() (string, error) {
	v, err := c.Kubectl.ServerVersion()
	if err != nil {
		return "", errors.Wrap(err, "failed to get kube server version")
	}

	return v.String(), nil
}

// DeploymentStatus returns running status for a Deployment
// If the deployment doesn't exist, the status is set to 0/0
func (c *Cluster) DeploymentStatus(namespace, selector string) (string, error) {
	result, err := c.Kubectl.AppsV1().Deployments(namespace).List(
		context.Background(),
		metav1.ListOptions{
			LabelSelector: selector,
		},
	)

	if err != nil {
		return "", errors.Wrap(err, "failed to get Deployment status")
	}

	if len(result.Items) < 1 {
		return "0/0", nil
	}

	return fmt.Sprintf("%d/%d", result.Items[0].Status.ReadyReplicas, result.Items[0].Status.Replicas), nil
}

// ListIngressRoutes returns a list of all routes for ingresses in `namespace` with the given selector
func (c *Cluster) ListIngressRoutes(namespace, name string) ([]string, error) {
	ingress, err := c.Kubectl.NetworkingV1().Ingresses(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list ingresses")
	}

	result := []string{}

	for _, rule := range ingress.Spec.Rules {
		result = append(result, rule.Host)
	}

	return result, nil
}

// ListIngress returns the list of available ingresses in `namespace` with the given selector
func (c *Cluster) ListIngress(namespace, selector string) (*v1beta1.IngressList, error) {
	listOptions := metav1.ListOptions{}
	if len(selector) > 0 {
		listOptions.LabelSelector = selector
	}

	// TODO: Switch to networking v1 when we don't care about <1.18 clusters
	ingressList, err := c.Kubectl.ExtensionsV1beta1().Ingresses(namespace).List(context.Background(), listOptions)
	if err != nil {
		return nil, err
	}
	return ingressList, nil
}

func (c *Cluster) execPod(namespace, podName, containerName string,
	command string, stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := []string{
		"sh",
		"-c",
		command,
	}
	req := c.Kubectl.CoreV1().RESTClient().Post().Resource("pods").Name(podName).
		Namespace(namespace).SubResource("exec")
	option := &v1.PodExecOptions{
		Container: containerName,
		Command:   cmd,
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       true,
	}
	if stdin == nil {
		option.Stdin = false
	}
	req.VersionedParams(
		option,
		scheme.ParameterCodec,
	)
	exec, err := remotecommand.NewSPDYExecutor(c.RestConfig, "POST", req.URL())
	if err != nil {
		return err
	}
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	})
	if err != nil {
		return err
	}

	return nil
}

// LabelNamespace adds a label to the namespace
func (c *Cluster) LabelNamespace(namespace, labelKey, labelValue string) error {
	patchContents := fmt.Sprintf(`{ "metadata": { "labels": { "%s": "%s" } } }`, labelKey, labelValue)

	_, err := c.Kubectl.CoreV1().Namespaces().Patch(context.Background(), namespace,
		types.StrategicMergePatchType, []byte(patchContents), metav1.PatchOptions{})

	if err != nil {
		return err
	}

	return nil
}

// NamespaceExistsAndOwned checks if the namespace exists
// and is created by fuseml or not.
func (c *Cluster) NamespaceExistsAndOwned(namespaceName string) (bool, error) {
	exists, err := c.NamespaceExists(namespaceName)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}

	owned, err := c.NamespaceLabelExists(namespaceName, FusemlDeploymentLabelKey)
	if err != nil {
		return false, err
	}
	return owned, nil
}

// NamespaceExistsAndNotOwned returns true only if the namespace exists
// but was NOT created by fuseml
func (c *Cluster) NamespaceExistsAndNotOwned(namespaceName string) (bool, error) {
	exists, err := c.NamespaceExists(namespaceName)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}

	owned, err := c.NamespaceLabelExists(namespaceName, FusemlDeploymentLabelKey)
	if err != nil {
		return false, err
	}
	return !owned, nil
}

// NamespaceExists checks if a namespace exists or not
func (c *Cluster) NamespaceExists(namespaceName string) (bool, error) {
	_, err := c.Kubectl.CoreV1().Namespaces().Get(context.Background(), namespaceName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// NamespaceLabelExists checks if a specific label exits on the namespace
func (c *Cluster) NamespaceLabelExists(namespaceName, labelKey string) (bool, error) {
	namespace, err := c.Kubectl.CoreV1().Namespaces().Get(context.Background(), namespaceName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	labelKey = fmt.Sprintf("%s/%s", APISGroupName, "deployment")
	if _, found := namespace.GetLabels()[labelKey]; found {
		return true, nil
	}

	return false, nil
}

// DeleteNamespace deletes the namepace
func (c *Cluster) DeleteNamespace(namespace string) error {
	err := c.Kubectl.CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	return nil
}

// HasIstio checks if istio is installed on the cluster
func (c *Cluster) HasIstio() bool {
	_, err := c.Kubectl.CoreV1().Services("istio-system").Get(
		context.Background(),
		"istiod",
		metav1.GetOptions{},
	)
	if err != nil {
		return false
	}
	return true
}

// HasKnative checks if Knative serving is installed on the cluster
func (c *Cluster) HasKnative() bool {
	_, err := c.Kubectl.CoreV1().Services("knative-serving").Get(
		context.Background(),
		"controller",
		metav1.GetOptions{},
	)
	if err != nil {
		return false
	}
	return true
}
