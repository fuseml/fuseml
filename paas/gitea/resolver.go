package gitea

import (
	"context"
	"fmt"
	"strings"

	"github.com/fuseml/fuseml/cli/deployments"
	"github.com/fuseml/fuseml/cli/kubernetes"
	"github.com/fuseml/fuseml/cli/paas/config"
	"github.com/pkg/errors"
	versionedclient "istio.io/client-go/pkg/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	GiteaCredentialsSecret = "gitea-creds"
)

// Resolver figures out where Gitea lives and how to login to it
type Resolver struct {
	cluster *kubernetes.Cluster
	config  *config.Config
}

// NewResolver creates a new Resolver
func NewResolver(config *config.Config, cluster *kubernetes.Cluster) *Resolver {
	return &Resolver{
		cluster: cluster,
		config:  config,
	}
}

// GetMainDomain finds the main domain for Fuseml
func (r *Resolver) GetMainDomain() (string, error) {
	var host string
	var err error
	if r.cluster.HasIstio() {
		host, err = r.getHostFromIstioGateway()
	} else {
		host, err = r.getHostFromIngress()
	}
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(host, "gitea."), nil
}

// GetGiteaURL finds the URL for gitea
func (r *Resolver) GetGiteaURL() (string, error) {
	var host string
	var err error
	if r.cluster.HasIstio() {
		host, err = r.getHostFromIstioGateway()
	} else {
		host, err = r.getHostFromIngress()
	}
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s://%s", r.config.GiteaProtocol, host), nil
}

// GetGiteaCredentials resolves Gitea's credentials
func (r *Resolver) GetGiteaCredentials() (string, string, error) {
	s, err := r.cluster.GetSecret(r.config.FusemlWorkloadsNamespace, GiteaCredentialsSecret)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to read gitea credentials")
	}

	username, ok := s.Data["username"]
	if !ok {
		return "", "", errors.Wrap(err, "username key not found in gitea credentials secret")
	}

	password, ok := s.Data["password"]
	if !ok {
		return "", "", errors.Wrap(err, "password key not found in gitea credentials secret")
	}

	return string(username), string(password), nil
}

func (r *Resolver) getHostFromIngress() (string, error) {
	// Get the ingress
	ingresses, err := r.cluster.ListIngress(deployments.GiteaDeploymentID, "app.kubernetes.io/name=gitea")
	if err != nil {
		return "", errors.Wrap(err, "failed to list ingresses for gitea")
	}

	if len(ingresses.Items) < 1 {
		return "", errors.New("gitea ingress not found")
	}

	if len(ingresses.Items) > 1 {
		return "", errors.New("more than one gitea ingress found")
	}

	if len(ingresses.Items[0].Spec.Rules) < 1 {
		return "", errors.New("gitea ingress has no rules")
	}

	if len(ingresses.Items[0].Spec.Rules) > 1 {
		return "", errors.New("gitea ingress has more than on rule")
	}

	return ingresses.Items[0].Spec.Rules[0].Host, nil
}

func (r *Resolver) getHostFromIstioGateway() (string, error) {
	ic, err := versionedclient.NewForConfig(r.cluster.RestConfig)
	if err != nil {
		return "", errors.Wrap(err, "Failed to create istio client")
	}

	// Get the gateway
	gateways, err := ic.NetworkingV1alpha3().Gateways("gitea").List(context.TODO(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=gitea"})
	if err != nil {
		return "", errors.Wrap(err, "failed to list istio gateways for gitea")
	}

	if len(gateways.Items) < 1 {
		return "", errors.New("gitea istio gateway not found")
	}

	if len(gateways.Items) > 1 {
		return "", errors.New("more than one gitea istio gateway found")
	}

	if len(gateways.Items[0].Spec.Servers) < 1 {
		return "", errors.New("gitea istio gateway has no servers")
	}

	if len(gateways.Items[0].Spec.Servers) > 1 {
		return "", errors.New("gitea istio gateway has more than one server")
	}

	if len(gateways.Items[0].Spec.Servers[0].Hosts) < 1 {
		return "", errors.New("gitea istio gateway has no host")
	}

	if len(gateways.Items[0].Spec.Servers[0].Hosts) > 1 {
		return "", errors.New("gitea istio gateway has more than one host")
	}

	return gateways.Items[0].Spec.Servers[0].Hosts[0], nil
}
