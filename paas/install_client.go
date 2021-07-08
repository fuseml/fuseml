package paas

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fuseml/fuseml/cli/deployments"
	"github.com/fuseml/fuseml/cli/helpers"
	"github.com/fuseml/fuseml/cli/kubernetes"
	"github.com/fuseml/fuseml/cli/paas/config"
	"github.com/fuseml/fuseml/cli/paas/ui"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DefaultTimeoutSec = 300
)

// InstallClient provides functionality for talking to Kubernetes for
// installing Fuseml on it.
type InstallClient struct {
	kubeClient *kubernetes.Cluster
	ui         *ui.UI
	config     *config.Config
	Log        logr.Logger
}

// Install deploys fuseml to the cluster.
func (c *InstallClient) Install(cmd *cobra.Command, options *kubernetes.InstallationOptions) error {
	log := c.Log.WithName("Install")
	log.Info("start")
	defer log.Info("return")
	details := log.V(1) // NOTE: Increment of level, not absolute.

	c.ui.Note().Msg("FuseML installing...")

	var err error
	details.Info("process cli options")
	options, err = options.Populate(kubernetes.NewCLIOptionsReader(cmd))
	if err != nil {
		return err
	}

	interactive, err := cmd.Flags().GetBool("interactive")
	if err != nil {
		return err
	}

	if interactive {
		details.Info("query user for options")
		options, err = options.Populate(kubernetes.NewInteractiveOptionsReader(os.Stdout, os.Stdin))
		if err != nil {
			return err
		}
	} else {
		details.Info("fill defaults into options")
		options, err = options.Populate(kubernetes.NewDefaultOptionsReader())
		if err != nil {
			return err
		}
	}

	details.Info("show option configuration")
	c.showInstallConfiguration(options)

	// TODO (post MVP): Run a validation phase which perform
	// additional checks on the values. For example range limits,
	// proper syntax of the string, etc. do it as pghase, and late
	// to report all problems at once, instead of early and
	// piecemal.

	deployment := deployments.Istio{Timeout: DefaultTimeoutSec}

	details.Info("deploy", "Deployment", deployment.ID())
	err = deployment.Deploy(c.kubeClient, c.ui, options.ForDeployment(deployment.ID()))
	if err != nil {
		return err
	}

	// Try to give a nip.io domain if the user didn't specify one
	domain, err := options.GetOpt("system_domain", "")
	if err != nil {
		return err
	}

	details.Info("ensure system-domain")
	err = c.fillInMissingSystemDomain(domain)
	if err != nil {
		return err
	}
	if domain.Value.(string) == "" {
		return errors.New("You didn't provide a system_domain and we were unable to setup a nip.io domain (couldn't find an ExternalIP)")
	}
	if c.kubeClient.HasKnative() {
		err = c.setDomainForKnative(domain.Value.(string))
		if err != nil {
			return err
		}
	}

	c.ui.Success().Msg("Created system_domain: " + domain.Value.(string))

	for _, deployment := range []kubernetes.Deployment{
		&deployments.Workloads{Timeout: DefaultTimeoutSec},
		&deployments.Gitea{Timeout: DefaultTimeoutSec},
		&deployments.Registry{Timeout: DefaultTimeoutSec},
		&deployments.Tekton{Timeout: DefaultTimeoutSec},
		&deployments.Core{Timeout: DefaultTimeoutSec},
	} {
		details.Info("deploy", "Deployment", deployment.ID())

		err := deployment.Deploy(c.kubeClient, c.ui, options.ForDeployment(deployment.ID()))
		if err != nil {
			return err
		}
	}
	if err := downloadFuseMLCLI(c.ui); err != nil {
		return err
	}

	extensions, err := options.GetOpt("extensions", "")
	if err != nil {
		return err
	}
	if err := c.InstallExtensions(extensions.Value.([]string), options); err != nil {
		return err
	}

	c.ui.Success().WithStringValue("System domain", domain.Value.(string)).Msg("FuseML installed.")

	return nil
}

// InstallExtensions installs given ML extensions
func (c *InstallClient) InstallExtensions(extensions []string, options *kubernetes.InstallationOptions) error {
	if len(extensions) == 0 {
		return nil
	}

	extensionRepo, err := options.GetOpt("extension_repository", "")
	if err != nil {
		return err
	}

	for _, name := range extensions {
		c.ui.Note().Msg(fmt.Sprintf("Installing extension '%s'...", name))

		extension := deployments.NewExtension(name, extensionRepo.Value.(string))

		err := extension.LoadDescription()
		if err != nil {
			return errors.New(fmt.Sprintf("Failed to load description file of extension %s: %s", name, err.Error()))
		}

		err = extension.Install(c.kubeClient, c.ui, options)
		if err != nil {
			return errors.New(fmt.Sprintf("Failed to install extension %s: %s", name, err.Error()))
		}
	}
	return nil
}

// UninstallExtensions uninstalls given ML extensions
func (c *InstallClient) UninstallExtensions(extensions []string, options *kubernetes.InstallationOptions) error {

	if len(extensions) == 0 {
		return nil
	}
	for _, name := range extensions {
		c.ui.Note().Msg(fmt.Sprintf("Removing extension '%s'...", name))

		repository := config.DefaultExtensionsLocation()
		extension := deployments.NewExtension(name, repository)

		err := extension.LoadDescription()
		if err != nil {
			return errors.New(fmt.Sprintf("Failed to load description file of extension %s: %s", name, err.Error()))
		}

		err = extension.Uninstall(c.kubeClient, c.ui, options)
		if err != nil {
			return errors.New(fmt.Sprintf("Failed to uninstall extension %s: %s", name, err.Error()))
		}
	}

	return nil
}

// Uninstall removes fuseml from the cluster.
func (c *InstallClient) Uninstall(cmd *cobra.Command, options *kubernetes.InstallationOptions) error {
	log := c.Log.WithName("Uninstall")
	log.Info("start")
	defer log.Info("return")
	details := log.V(1) // NOTE: Increment of level, not absolute.

	var err error
	details.Info("process cli options")
	options, err = options.Populate(kubernetes.NewCLIOptionsReader(cmd))
	if err != nil {
		return err
	}

	extensions, err := options.GetOpt("extensions", "")
	if err != nil {
		return err
	}
	if err := c.UninstallExtensions(extensions.Value.([]string), options); err != nil {
		return err
	}

	c.ui.Note().Msg("FuseML uninstalling...")

	for _, deployment := range []kubernetes.Deployment{
		&deployments.Workloads{Timeout: DefaultTimeoutSec},
		&deployments.Tekton{Timeout: DefaultTimeoutSec},
		&deployments.Registry{Timeout: DefaultTimeoutSec},
		&deployments.Gitea{Timeout: DefaultTimeoutSec},
		&deployments.Core{Timeout: DefaultTimeoutSec},
		&deployments.Istio{Timeout: DefaultTimeoutSec},
	} {
		details.Info("remove", "Deployment", deployment.ID())
		err := deployment.Delete(c.kubeClient, c.ui)
		if err != nil {
			return err
		}
	}

	c.ui.Success().Msg("FuseML uninstalled.")

	return nil
}

func (c *InstallClient) Upgrade(cmd *cobra.Command, options *kubernetes.InstallationOptions) error {
	log := c.Log.WithName("Upgrade")
	log.Info("start")
	defer log.Info("return")
	details := log.V(1)

	c.ui.Note().Msg("FuseML upgrading...")

	options, err := options.Populate(kubernetes.NewCLIOptionsReader(cmd))
	if err != nil {
		return err
	}

	for _, deployment := range []kubernetes.Deployment{
		&deployments.Core{Timeout: DefaultTimeoutSec},
	} {
		details.Info("upgrade", "Deployment", deployment.ID())
		err := deployment.Upgrade(c.kubeClient, c.ui, options.ForDeployment(deployment.ID()))
		if err != nil {
			return err
		}
	}

	c.ui.Success().Msg("FuseML upgraded.")

	return nil
}

// showInstallConfiguration prints the options and their values to stdout, to
// inform the user of the detected and chosen configuration
func (c *InstallClient) showInstallConfiguration(opts *kubernetes.InstallationOptions) {
	m := c.ui.Normal()
	for _, opt := range *opts {
		name := "  :compass: " + opt.Name
		switch opt.Type {
		case kubernetes.BooleanType:
			m = m.WithBoolValue(name, opt.Value.(bool))
		case kubernetes.StringType:
			m = m.WithStringValue(name, opt.Value.(string))
		case kubernetes.IntType:
			m = m.WithIntValue(name, opt.Value.(int))
		}
	}
	m.Msg("Configuration...")
}

func (c *InstallClient) fillInMissingSystemDomain(domain *kubernetes.InstallationOption) error {
	if domain.Value.(string) == "" {
		service := "traefik"
		if c.kubeClient.HasIstio() {
			service = "istio-ingressgateway"
		}
		ip := ""
		s := c.ui.Progressf(fmt.Sprintf("Waiting for LoadBalancer IP on %s service.", service))
		defer s.Stop()
		err := helpers.RunToSuccessWithTimeout(
			func() error {
				return c.fetchIP(&ip, service)
			}, time.Duration(2)*time.Minute, 3*time.Second)
		if err != nil {
			if strings.Contains(err.Error(), "Timed out after") {
				return errors.New("Timed out waiting for LoadBalancer IP on " + service + " service.\n" +
					"Ensure your kubernetes platform has the ability to provision LoadBalancer IP address.\n\n" +
					"Follow these steps to enable this ability\n" +
					"https://github.com/fuseml/fuseml/blob/main/docs/install.md")
			}
			return err
		}

		if ip != "" {
			domain.Value = fmt.Sprintf("%s.nip.io", ip)
		}

	}

	return nil
}

func (c *InstallClient) fetchIP(ip *string, service string) error {
	serviceList, err := c.kubeClient.Kubectl.CoreV1().Services("").List(context.Background(), metav1.ListOptions{
		FieldSelector: "metadata.name=" + service,
	})
	if len(serviceList.Items) == 0 {
		return errors.New(fmt.Sprintf("couldn't find the %s service", service))
	}
	if err != nil {
		return err
	}
	ingress := serviceList.Items[0].Status.LoadBalancer.Ingress
	if len(ingress) <= 0 {
		return errors.New(fmt.Sprintf("ingress list is empty in %s service", service))
	}
	*ip = ingress[0].IP

	return nil
}

func (c *InstallClient) setDomainForKnative(domain string) error {
	knDomainConfig, err := c.kubeClient.Kubectl.CoreV1().ConfigMaps("knative-serving").Get(context.Background(), "config-domain", metav1.GetOptions{})
	if err != nil {
		return err
	}
	knDomainConfig.Data[domain] = ""
	_, err = c.kubeClient.Kubectl.CoreV1().ConfigMaps("knative-serving").Update(context.Background(), knDomainConfig, metav1.UpdateOptions{})
	if err != nil {
		return errors.New("could not update Knative domain configuration")
	}
	return nil
}
