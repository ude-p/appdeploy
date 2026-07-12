package controller

import (
	"context"
	"encoding/json"
	"fmt"

	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appdeployv1 "github.com/ude-p/appdeploy/api/v1"
)

var ingressOverrideAllowlist = map[string]struct{}{
	"metadata.labels": {},
}

func (r *AppDeployReconciler) reconcileIngress(ctx context.Context, namespace string, ingress *appdeployv1.AppDeployIngress) error {
	ing := &networkingv1.Ingress{}
	key := client.ObjectKey{Name: ingress.Name, Namespace: namespace}
	err := r.Get(ctx, key, ing)
	if apierrors.IsNotFound(err) {
		ing = &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:        ingress.Name,
				Namespace:   namespace,
				Annotations: ingress.Annotations,
			},
			Spec: networkingv1.IngressSpec{
				IngressClassName: &ingress.ClassName,
				Rules:            buildIngressRules(ingress),
			},
		}
		if ingress.TLSSecretName != "" {
			ing.Spec.TLS = []networkingv1.IngressTLS{
				{
					SecretName: ingress.TLSSecretName,
					Hosts:      []string{ingress.Host},
				},
			}
		}
		if err := applyIngressOverrides(ing, ingress.Overrides.Raw); err != nil {
			return err
		}
		return r.Create(ctx, ing)
	}
	if err != nil {
		return err
	}

	ing.Annotations = ingress.Annotations
	ing.Spec.IngressClassName = &ingress.ClassName
	ing.Spec.Rules = buildIngressRules(ingress)
	if ingress.TLSSecretName != "" {
		ing.Spec.TLS = []networkingv1.IngressTLS{
			{
				SecretName: ingress.TLSSecretName,
				Hosts:      []string{ingress.Host},
			},
		}
	} else {
		ing.Spec.TLS = nil
	}
	if err := applyIngressOverrides(ing, ingress.Overrides.Raw); err != nil {
		return err
	}

	return r.Update(ctx, ing)
}

func buildIngressRules(ingress *appdeployv1.AppDeployIngress) []networkingv1.IngressRule {
	rules := make([]networkingv1.IngressRule, 0, len(ingress.Rules))
	httpPaths := make([]networkingv1.HTTPIngressPath, 0, len(ingress.Rules))
	for _, rule := range ingress.Rules {
		httpPaths = append(httpPaths, networkingv1.HTTPIngressPath{
			Path:     rule.Path,
			PathType: pathTypePtr(networkingv1.PathTypePrefix),
			Backend: networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{
					Name: rule.ServiceName,
					Port: networkingv1.ServiceBackendPort{},
				},
			},
		})
		if len(httpPaths) > 0 {
			backend := &httpPaths[len(httpPaths)-1].Backend
			if rule.ServicePort.Type == intstr.Int {
				backend.Service.Port.Number = int32(rule.ServicePort.IntValue())
			} else {
				backend.Service.Port.Name = rule.ServicePort.StrVal
			}
		}
	}
	if len(httpPaths) > 0 {
		rules = append(rules, networkingv1.IngressRule{
			Host: ingress.Host,
			IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{
					Paths: httpPaths,
				},
			},
		})
	}
	return rules
}

func pathTypePtr(pathType networkingv1.PathType) *networkingv1.PathType {
	return new(pathType)
}

func applyIngressOverrides(ingress *networkingv1.Ingress, raw []byte) error {
	if len(raw) == 0 {
		return nil
	}

	var overrides map[string]json.RawMessage
	if err := json.Unmarshal(raw, &overrides); err != nil {
		return fmt.Errorf("invalid ingress overrides: %w", err)
	}

	for key := range overrides {
		if _, ok := ingressOverrideAllowlist[key]; !ok {
			return fmt.Errorf("ingress override path %q is not allowed", key)
		}
	}

	for key, value := range overrides {
		switch key {
		case "metadata.labels":
			if err := json.Unmarshal(value, &ingress.Labels); err != nil {
				return fmt.Errorf("invalid ingress override %q: %w", key, err)
			}
		}
	}

	return nil
}
