package render

import (
	"fmt"
	"maps"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/ude-p/appdeploy/internal/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// servicePorts maps declared ports to Service ports. Kubernetes requires every
// port of a multi-port Service to be named, so a name is generated when there
// is more than one.
func servicePorts(ports []int32) []corev1.ServicePort {
	servicePorts := make([]corev1.ServicePort, 0, len(ports))
	for _, port := range ports {
		servicePort := corev1.ServicePort{
			Port:       port,
			TargetPort: intstr.FromInt32(port),
		}
		if len(ports) > 1 {
			servicePort.Name = fmt.Sprintf("port-%d", port)
		}
		servicePorts = append(servicePorts, servicePort)
	}
	return servicePorts
}

func nodeSelector(model config.Model, value map[string]string) map[string]string {
	if len(value) > 0 {
		return value
	}
	if model.Defaults != nil {
		return model.Defaults.NodeSelector
	}
	return nil
}

func configureProbes(container *corev1.Container, health *config.Health, ports []int32) {
	var probe *corev1.Probe
	if health != nil {
		port := health.Port
		if port == 0 {
			port = ports[0]
		}
		probe = &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{Path: health.Path, Port: intstr.FromInt32(port)},
		}}
	} else if len(ports) > 0 {
		probe = &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(ports[0])},
		}}
	}
	if probe != nil {
		container.LivenessProbe = probe
		container.ReadinessProbe = probe.DeepCopy()
	}
}

func downwardEnv() []corev1.EnvVar {
	names := slices.Sorted(maps.Keys(config.DownwardEnv))
	fields := make([]corev1.EnvVar, 0, len(names))
	for _, name := range names {
		fields = append(fields, corev1.EnvVar{
			Name: name,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: config.DownwardEnv[name]},
			},
		})
	}
	return fields
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
