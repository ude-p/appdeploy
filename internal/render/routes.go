package render

import (
	"fmt"

	"github.com/ude-p/appdeploy/internal/config"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func buildIngresses(model config.Model, namespace string) ([]client.Object, error) {
	objects := make([]client.Object, 0, len(model.Routes))
	for _, route := range model.Routes {
		appName, err := referenceName(route.To)
		if err != nil {
			return nil, fmt.Errorf("route %q: %w", route.Name, err)
		}
		appPorts, err := appServicePorts(model.Apps, appName)
		if err != nil {
			return nil, fmt.Errorf("route %q: %w", route.Name, err)
		}
		prefix := networkingv1.PathTypePrefix
		paths := route.Paths
		if len(paths) == 0 {
			paths = []config.RoutePath{{Path: "/"}}
		}
		ingressPaths := make([]networkingv1.HTTPIngressPath, 0, len(paths))
		for _, path := range paths {
			port := appPorts[0]
			if path.Port != nil {
				port = *path.Port
			}
			ingressPaths = append(ingressPaths, networkingv1.HTTPIngressPath{
				Path:     path.Path,
				PathType: &prefix,
				Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{
					Name: appName,
					Port: networkingv1.ServiceBackendPort{Number: port},
				}},
			})
		}

		ingress := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: route.Name, Namespace: namespace},
			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{{
					Host: route.Host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{Paths: ingressPaths},
					},
				}},
			},
		}
		if route.ClassName != "" {
			ingress.Spec.IngressClassName = &route.ClassName
		}
		if len(route.Annotations) > 0 {
			ingress.Annotations = route.Annotations
		}
		if route.TLSSecret != "" {
			ingress.Spec.TLS = []networkingv1.IngressTLS{{
				Hosts:      []string{route.Host},
				SecretName: route.TLSSecret,
			}}
		}
		objects = append(objects, ingress)
	}
	return objects, nil
}

func appServicePorts(apps []config.App, name string) ([]int32, error) {
	for _, app := range apps {
		if app.Name == name && len(app.Ports) > 0 {
			ports := make([]int32, 0, len(app.Ports))
			for _, port := range app.Ports {
				ports = append(ports, port.Service)
			}
			return ports, nil
		}
	}
	return nil, fmt.Errorf("app %q has no service port", name)
}
