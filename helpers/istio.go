package helpers

import (
	"fmt"
	"io/ioutil"
	"os"
	"text/template"
)

// CreateIstioIngressGateway creates a temporary ingress gateway definition for teh specified service
func CreateIstioIngressGateway(name string, namespace string, host string, serviceHost string, servicePort int) (string, error) {

	tmpFile, err := ioutil.TempFile("", "fluo")
	if err != nil {
		return tmpFile.Name(), err
	}
	defer os.Remove(tmpFile.Name())

	istioGatewayTmpl, err := template.New("istiogw").Parse(`
---
apiVersion: networking.istio.io/v1alpha3
kind: Gateway
metadata:
  name: {{ .Name }}-gateway
  namespace: {{ .Namespace }}
  labels:
    "app.kubernetes.io/name": {{ .Name }}
spec:
  selector:
    istio: ingressgateway # use Istio default gateway implementation
  servers:
  - port:
      number: 80
      name: http
      protocol: HTTP
    hosts:
    - "{{ .Host }}"

---
apiVersion: networking.istio.io/v1alpha3
kind: VirtualService
metadata:
  name: {{ .Name }}
  namespace: {{ .Namespace }}
spec:
  hosts:
  - "{{ .Host }}"
  gateways:
  - {{ .Name }}-gateway
  http:
  - match:
    - uri:
        prefix: /
    route:
    - destination:
        port:
          number: {{ .ServicePort }}
        host: {{ .ServiceHost }}
`)
	if err != nil {
		return tmpFile.Name(), err
	}

	err = istioGatewayTmpl.Execute(tmpFile, struct {
		Name        string
		Namespace   string
		Host        string
		ServiceHost string
		ServicePort int
	}{
		Name:        name,
		Namespace:   namespace,
		Host:        host,
		ServiceHost: serviceHost,
		ServicePort: servicePort,
	})
	if err != nil {
		return tmpFile.Name(), err
	}

	return Kubectl(fmt.Sprintf("apply --filename %s", tmpFile.Name()))
}
