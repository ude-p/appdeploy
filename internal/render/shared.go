package render

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/ude-p/appdeploy/internal/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// servicePorts maps declared ports to Service ports. Kubernetes requires every
// port of a multi-port Service to be named, so a name is generated when there
// is more than one.
func servicePorts(ports []config.Port) []corev1.ServicePort {
	servicePorts := make([]corev1.ServicePort, 0, len(ports))
	for _, port := range ports {
		containerPort := port.Container
		if containerPort == 0 {
			containerPort = port.Service
		}
		servicePort := corev1.ServicePort{
			Port:       port.Service,
			TargetPort: intstr.FromInt32(containerPort),
		}
		if len(ports) > 1 {
			servicePort.Name = fmt.Sprintf("port-%d", port.Service)
		}
		servicePorts = append(servicePorts, servicePort)
	}
	return servicePorts
}

func referenceName(expression hcl.Expression) (string, error) {
	traversal, diagnostics := hcl.AbsTraversalForExpr(expression)
	if diagnostics.HasErrors() || len(traversal) != 2 {
		return "", fmt.Errorf("must be a resource reference")
	}
	name, ok := traversal[1].(hcl.TraverseAttr)
	if !ok {
		return "", fmt.Errorf("must be a resource reference")
	}
	return name.Name, nil
}
