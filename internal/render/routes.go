package render

import (
	"maps"

	"github.com/ude-p/appdeploy/internal/config"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func buildIngresses(model config.Model, namespace string) ([]client.Object, error) {
	objects := make([]client.Object, 0)
	for _, app := range model.Apps {
		if app.Scope != "" && app.Scope != namespace {
			continue
		}
		for _, route := range app.Routes {
			if route.Scope != "" && route.Scope != namespace {
				continue
			}
			paths := route.Paths
			if route.Path != "" {
				paths = []string{route.Path}
			}
			if len(paths) == 0 {
				paths = []string{"/"}
			}
			port := int32(0)
			if route.Port != nil {
				port = *route.Port
			}
			if port == 0 {
				port = app.ServicePorts()[0]
			}

			prefix := networkingv1.PathTypePrefix
			ingressPaths := make([]networkingv1.HTTPIngressPath, 0, len(paths))
			for _, path := range paths {
				ingressPaths = append(ingressPaths, networkingv1.HTTPIngressPath{
					Path:     path,
					PathType: &prefix,
					Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{
						Name: app.Name,
						Port: networkingv1.ServiceBackendPort{Number: port},
					}},
				})
			}

			ingress := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:        route.Name,
					Namespace:   namespace,
					Annotations: ingressAnnotations(model, route),
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{{
						Host: route.Host,
						IngressRuleValue: networkingv1.IngressRuleValue{
							HTTP: &networkingv1.HTTPIngressRuleValue{Paths: ingressPaths},
						},
					}},
				},
			}
			if class := ingressClass(model); class != "" {
				ingress.Spec.IngressClassName = &class
			}
			if route.TLS != "" {
				ingress.Spec.TLS = []networkingv1.IngressTLS{{
					Hosts:      []string{route.Host},
					SecretName: route.TLS,
				}}
			}
			objects = append(objects, ingress)
		}
	}
	return objects, nil
}

func ingressClass(model config.Model) string {
	if model.Defaults != nil {
		return model.Defaults.IngressClass
	}
	return ""
}

func ingressAnnotations(model config.Model, route config.Route) map[string]string {
	var annotations map[string]string
	if model.Defaults != nil {
		annotations = maps.Clone(model.Defaults.IngressAnnotations)
	}
	for name, value := range route.Annotations {
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations[name] = value
	}
	return annotations
}
