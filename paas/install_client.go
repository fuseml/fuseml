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

	if err := downloadFuseMLCLI(c.ui, domain.Value.(string)); err != nil {
		return err
	}

	extensions, err := options.GetOpt("extensions", "")
	if err != nil {
		return err
	}
	details.Info("installing extensions")
	if err := c.handleExtensions("install", extensions.Value.([]string), options, true); err != nil {
		return err
	}

	c.ui.Success().WithStringValue("System domain", domain.Value.(string)).Msg("FuseML installed.")

	return nil
}

// find out the required extensions for an extension that is passed as an argument
// return list of all requirements, including the given extension itself
func getRequirementsForExtension(extension *deployments.Extension, repo string) ([]*deployments.Extension, error) {

	ret := []*deployments.Extension{}
	name := extension.Name
	err := extension.LoadDescription()
	if err != nil {
		return ret, errors.New(fmt.Sprintf("Failed to load description file of required extension %s: %s", name, err.Error()))
	}

	for _, req := range extension.Desc.Requires {

		reqExt := deployments.NewExtension(req, repo, DefaultTimeoutSec)
		sortedRequiredExtensions, _ := getRequirementsForExtension(reqExt, repo)

		for _, e := range sortedRequiredExtensions {
			ret = append(ret, e)
		}
	}
	ret = append(ret, extension)
	return ret, nil
}

// install or uninstall given list of extensions
func (c *InstallClient) handleExtensions(action string, extensions []string, options *kubernetes.InstallationOptions, withDeps bool) error {

	if len(extensions) == 0 {
		return nil
	}

	extensionRepo, err := options.GetOpt("extension_repository", "")
	if err != nil {
		return err
	}
	// we do not know the size as the list of needs to be eventually installed could be bigger than original
	sortedExtensions := []*deployments.Extension{}
	// remember what we've already added to the sorted queue and avoid duplicates
	exensionsInQueue := make(map[string]bool)

	// in first loop, go over extensions and find their dependencies
	for _, name := range extensions {

		extension := deployments.NewExtension(name, extensionRepo.Value.(string), DefaultTimeoutSec)

		requiredExtensions, err := getRequirementsForExtension(extension, extensionRepo.Value.(string))
		if err != nil {
			return err
		}

		for _, e := range requiredExtensions {
			if !exensionsInQueue[e.Name] {
				if action == "install" {
					sortedExtensions = append(sortedExtensions, e)
				} else {
					// for uninstallation, the order must be reversed
					sortedExtensions = append([]*deployments.Extension{e}, sortedExtensions...)
				}
				exensionsInQueue[e.Name] = true
			}
		}
	}

	for _, extension := range sortedExtensions {

		switch action {
		case "install":
			c.ui.Note().Msg(fmt.Sprintf("Installing extension '%s'...", extension.Name))
			err = extension.Install(c.kubeClient, c.ui, options)
			if err != nil {
				return errors.New(fmt.Sprintf("Failed to install extension %s: %s", extension.Name, err.Error()))
			}

			c.ui.Note().Msg(fmt.Sprintf("Registering extension '%s'...", extension.Name))
			err = extension.Register(c.kubeClient, c.ui, options)
			if err != nil {
				return errors.New(fmt.Sprintf("Failed to register extension %s: %s", extension.Name, err.Error()))
			}
		case "uninstall":
			// uninstall dependencies only when explicitly required on command line or with the command that uninstalls whole fuseml
			// (https://github.com/fuseml/fuseml/issues/198)
			if withDeps || helpers.StringInSlice(extensions, extension.Name) {
				c.ui.Note().Msg(fmt.Sprintf("Unregistering extension '%s'...", extension.Name))
				err = extension.UnRegister(c.kubeClient, c.ui, options)
				if err != nil {
					return errors.New(fmt.Sprintf("Failed to unregister extension %s: %s", extension.Name, err.Error()))
				}

				c.ui.Note().Msg(fmt.Sprintf("Removing extension '%s'...", extension.Name))
				err = extension.Uninstall(c.kubeClient, c.ui, options)
				if err != nil {
					return errors.New(fmt.Sprintf("Failed to uninstall extension %s: %s", extension.Name, err.Error()))
				}
			} else {
				c.ui.Note().Msg(fmt.Sprintf("Skipped removal of extension '%s'", extension.Name))
			}
		default:
			return errors.New(fmt.Sprintf("Unsupported action %s", action))
		}
	}
	return nil
}

func (c *InstallClient) listRegisteredExtensions(options *kubernetes.InstallationOptions) error {

	exts, err := deployments.GetRegisteredExtensions(options)
	if err != nil {
		return err
	}
	msg := c.ui.Success().WithTable("Name", "Version", "Description")
	for _, ext := range exts {
		msg = msg.WithTableRow(*ext.ID, *ext.Version, *ext.Description)
	}
	msg.Msg("Registered FuseML extensions:")

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
	details.Info("fill defaults into options")
	options, err = options.Populate(kubernetes.NewDefaultOptionsReader())
	if err != nil {
		return err
	}

	details.Info("show option configuration")
	c.showInstallConfiguration(options)

	domain, err := options.GetOpt("system_domain", "")
	if err != nil {
		return err
	}

	details.Info("ensure system-domain")
	err = c.fillInMissingSystemDomain(domain)
	if err != nil {
		return err
	}

	extensions, err := options.GetOpt("extensions", "")
	if err != nil {
		return err
	}
	details.Info("removing extensions")
	if err := c.handleExtensions("uninstall", extensions.Value.([]string), options, true); err != nil {
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

func (c *InstallClient) Extensions(cmd *cobra.Command, options *kubernetes.InstallationOptions) error {
	log := c.Log.WithName("Extensions")
	log.Info("start")
	defer log.Info("return")
	details := log.V(1)

	c.ui.Note().Msg("FuseML handling the extensions...")

	details.Info("process cli options")
	options, err := options.Populate(kubernetes.NewCLIOptionsReader(cmd))
	if err != nil {
		return err
	}
	details.Info("fill defaults into options")
	options, err = options.Populate(kubernetes.NewDefaultOptionsReader())
	if err != nil {
		return err
	}

	domain, err := options.GetOpt("system_domain", "")
	if err != nil {
		return err
	}

	details.Info("ensure system-domain")
	err = c.fillInMissingSystemDomain(domain)
	if err != nil {
		return err
	}

	addExtensions, err := options.GetOpt("add", "")
	if err != nil {
		return err
	}

	details.Info("installing extensions")
	if err := c.handleExtensions("install", addExtensions.Value.([]string), options, true); err != nil {
		return err
	}

	removeExtensions, err := options.GetOpt("remove", "")
	if err != nil {
		return err
	}

	withDeps, err := options.GetBool("with_dependencies", "")
	if err != nil {
		return err
	}

	details.Info("removing extensions")
	if err := c.handleExtensions("uninstall", removeExtensions.Value.([]string), options, withDeps); err != nil {
		return err
	}

	doList, err := options.GetBool("list", "")
	if err != nil {
		return err
	}
	if doList {
		details.Info("listing extensions")
		if err := c.listRegisteredExtensions(options); err != nil {
			return err
		}
	}

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
