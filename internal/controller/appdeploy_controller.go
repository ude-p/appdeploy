/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appdeployv1alpha1 "github.com/ude-p/appdeploy/api/v1alpha1"
)

// AppDeployReconciler reconciles a AppDeploy object
type AppDeployReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

var deploymentOverrideAllowlist = map[string]struct{}{
	"metadata.annotations":               {},
	"metadata.labels":                    {},
	"spec.strategy":                      {},
	"spec.template.metadata.annotations": {},
	"spec.template.spec.securityContext": {},
}

var ingressOverrideAllowlist = map[string]struct{}{
	"metadata.labels": {},
}

// +kubebuilder:rbac:groups=appdeploy.appdeploy.io,resources=appdeploys,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=appdeploy.appdeploy.io,resources=appdeploys/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=appdeploy.appdeploy.io,resources=appdeploys/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=external-secrets.io,resources=externalsecrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AppDeploy object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.24.1/pkg/reconcile
func (r *AppDeployReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var appdeploy appdeployv1alpha1.AppDeploy
	if err := r.Get(ctx, req.NamespacedName, &appdeploy); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if err := r.validate(&appdeploy); err != nil {
		return ctrl.Result{}, err
	}

	targetNamespaces := appdeploy.Spec.Namespaces
	if len(appdeploy.Spec.SelectedNamespaces) > 0 {
		targetNamespaces = appdeploy.Spec.SelectedNamespaces
	}

	for _, namespace := range targetNamespaces {
		for i := range appdeploy.Spec.ConfigMaps {
			configMap := appdeploy.Spec.ConfigMaps[i]
			if configMap.Scope != "" && configMap.Scope != namespace {
				continue
			}
			if err := r.reconcileConfigMap(ctx, namespace, &configMap); err != nil {
				return ctrl.Result{}, err
			}
		}

		for i := range appdeploy.Spec.Secrets {
			secret := appdeploy.Spec.Secrets[i]
			if secret.Scope != "" && secret.Scope != namespace {
				continue
			}
			if err := r.reconcileExternalSecret(ctx, namespace, &secret); err != nil {
				return ctrl.Result{}, err
			}
		}

		for i := range appdeploy.Spec.Workloads {
			workload := appdeploy.Spec.Workloads[i]
			if workload.Scope != "" && workload.Scope != namespace {
				continue
			}
			switch workload.Kind {
			case "", "Deployment":
				if err := r.reconcileDeployment(ctx, namespace, &workload); err != nil {
					return ctrl.Result{}, err
				}
			case "StatefulSet":
				if err := r.reconcileStatefulSet(ctx, namespace, &workload); err != nil {
					return ctrl.Result{}, err
				}
			default:
				return ctrl.Result{}, fmt.Errorf("spec.workloads[%d].kind %q is not supported", i, workload.Kind)
			}
			if err := r.reconcileService(ctx, namespace, &workload); err != nil {
				return ctrl.Result{}, err
			}
		}

		for i := range appdeploy.Spec.Ingresses {
			ingress := appdeploy.Spec.Ingresses[i]
			if ingress.Scope != "" && ingress.Scope != namespace {
				continue
			}
			if err := r.reconcileIngress(ctx, namespace, &ingress); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *AppDeployReconciler) validate(appdeploy *appdeployv1alpha1.AppDeploy) error {
	if len(appdeploy.Spec.Namespaces) == 0 {
		return fmt.Errorf("spec.namespaces must not be empty")
	}

	namespaceSet := make(map[string]struct{}, len(appdeploy.Spec.Namespaces))
	for i, input := range appdeploy.Spec.Namespaces {
		if _, ok := namespaceSet[input]; ok {
			return fmt.Errorf("spec.namespaces[%d] duplicates namespace %q", i, input)
		}
		namespaceSet[input] = struct{}{}
	}

	selectedNamespaceSet := make(map[string]struct{}, len(appdeploy.Spec.SelectedNamespaces))
	for i, selectedNamespace := range appdeploy.Spec.SelectedNamespaces {
		if _, ok := selectedNamespaceSet[selectedNamespace]; ok {
			return fmt.Errorf("spec.selectedNamespaces[%d] duplicates namespace %q", i, selectedNamespace)
		}
		selectedNamespaceSet[selectedNamespace] = struct{}{}
		if _, ok := namespaceSet[selectedNamespace]; !ok {
			return fmt.Errorf("spec.selectedNamespaces[%d] %q must match one of spec.namespaces", i, selectedNamespace)
		}
	}

	for i, workload := range appdeploy.Spec.Workloads {
		if workload.Scope != "" {
			if _, ok := namespaceSet[workload.Scope]; !ok {
				return fmt.Errorf("spec.workloads[%d].scope %q must match one of spec.namespaces", i, workload.Scope)
			}
		}
		if workload.Kind == "StatefulSet" && workload.HeadlessServiceName == "" {
			return fmt.Errorf("spec.workloads[%d].headlessServiceName must be set when kind is StatefulSet", i)
		}
		for j, volumeMount := range workload.VolumeMounts {
			if volumeMount.ConfigMapName == "" && volumeMount.SecretName == "" {
				return fmt.Errorf("spec.workloads[%d].volumeMounts[%d] must set configMapName or secretName", i, j)
			}
			if volumeMount.ConfigMapName != "" && volumeMount.SecretName != "" {
				return fmt.Errorf("spec.workloads[%d].volumeMounts[%d] must not set both configMapName and secretName", i, j)
			}
		}
	}

	for i, configMap := range appdeploy.Spec.ConfigMaps {
		if configMap.Scope != "" {
			if _, ok := namespaceSet[configMap.Scope]; !ok {
				return fmt.Errorf("spec.configMaps[%d].scope %q must match one of spec.namespaces", i, configMap.Scope)
			}
		}
	}

	for i, secret := range appdeploy.Spec.Secrets {
		if secret.Scope != "" {
			if _, ok := namespaceSet[secret.Scope]; !ok {
				return fmt.Errorf("spec.secrets[%d].scope %q must match one of spec.namespaces", i, secret.Scope)
			}
		}
		switch secret.SecretStoreKind {
		case "SecretStore", "ClusterSecretStore":
		default:
			return fmt.Errorf("spec.secrets[%d].secretStoreKind %q is not supported", i, secret.SecretStoreKind)
		}
	}

	for i, ingress := range appdeploy.Spec.Ingresses {
		if ingress.Scope != "" {
			if _, ok := namespaceSet[ingress.Scope]; !ok {
				return fmt.Errorf("spec.ingresses[%d].scope %q must match one of spec.namespaces", i, ingress.Scope)
			}
		}
	}
	return nil
}

func (r *AppDeployReconciler) reconcileConfigMap(ctx context.Context, namespace string, configMap *appdeployv1alpha1.AppDeployConfigMap) error {
	cm := &corev1.ConfigMap{}
	key := client.ObjectKey{Name: configMap.Name, Namespace: namespace}
	err := r.Get(ctx, key, cm)
	if apierrors.IsNotFound(err) {
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMap.Name,
				Namespace: namespace,
			},
			Data: configMap.Data,
		}
		return r.Create(ctx, cm)
	}
	if err != nil {
		return err
	}

	cm.Data = configMap.Data

	return r.Update(ctx, cm)
}

func (r *AppDeployReconciler) reconcileExternalSecret(ctx context.Context, namespace string, secret *appdeployv1alpha1.AppDeploySecret) error {
	gvk := schema.GroupVersionKind{
		Group:   "external-secrets.io",
		Version: "v1beta1",
		Kind:    "ExternalSecret",
	}

	targetType := "Opaque"
	if secret.Type == "ImagePull" {
		targetType = "kubernetes.io/dockerconfigjson"
	}

	externalSecret := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "external-secrets.io/v1beta1",
			"kind":       "ExternalSecret",
			"metadata": map[string]any{
				"name":      secret.Name,
				"namespace": namespace,
			},
			"spec": map[string]any{
				"secretStoreRef": map[string]any{
					"name": secret.SecretStoreName,
					"kind": secret.SecretStoreKind,
				},
				"target": map[string]any{
					"name":           secret.Name,
					"creationPolicy": "Owner",
					"template": map[string]any{
						"type": targetType,
					},
				},
				"dataFrom": []any{
					map[string]any{
						"extract": map[string]any{
							"key": secret.RemoteKey,
						},
					},
				},
			},
		},
	}
	externalSecret.SetGroupVersionKind(gvk)

	key := client.ObjectKey{Name: secret.Name, Namespace: namespace}
	current := &unstructured.Unstructured{}
	current.SetGroupVersionKind(gvk)
	err := r.Get(ctx, key, current)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, externalSecret)
	}
	if err != nil {
		return err
	}

	current.Object["spec"] = externalSecret.Object["spec"]
	return r.Update(ctx, current)
}

func (r *AppDeployReconciler) reconcileDeployment(ctx context.Context, namespace string, workload *appdeployv1alpha1.AppDeployWorkload) error {
	name := workload.Name
	replicas := int32(1)
	if workload.Replicas != nil {
		replicas = *workload.Replicas
	}
	envFrom := buildEnvFromSources(workload)
	imagePullSecrets := buildImagePullSecrets(workload)
	policy := imagePullPolicy(workload)
	volumeMounts := buildVolumeMounts(workload)
	volumes := buildVolumes(workload)
	resources := workload.Resources

	deployment := &appsv1.Deployment{}
	key := client.ObjectKey{Name: name, Namespace: namespace}

	err := r.Get(ctx, key, deployment)
	if apierrors.IsNotFound(err) {
		deployment = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"appdeploy.appdeploy.io/workload": name,
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"appdeploy.appdeploy.io/workload": name,
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:            name,
								Image:           workload.Image,
								ImagePullPolicy: policy,
								EnvFrom:         envFrom,
								VolumeMounts:    volumeMounts,
								Resources:       resources,
								Ports: []corev1.ContainerPort{
									{
										ContainerPort: workload.ContainerPort,
										Protocol:      corev1.ProtocolTCP,
									},
								},
							},
						},
						ImagePullSecrets: imagePullSecrets,
						Volumes:          volumes,
					},
				},
				},
			}
			if err := applyDeploymentOverrides(deployment, workload.Overrides.Raw); err != nil {
				return err
			}
			return r.Create(ctx, deployment)
		}
	if err != nil {
		return err
	}

	deployment.Spec.Replicas = &replicas
	deployment.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"appdeploy.appdeploy.io/workload": name,
		},
	}
	deployment.Spec.Template.Labels = map[string]string{
		"appdeploy.appdeploy.io/workload": name,
	}
	if len(deployment.Spec.Template.Spec.Containers) == 0 {
		deployment.Spec.Template.Spec.Containers = []corev1.Container{{Name: name}}
	}
	deployment.Spec.Template.Spec.Containers[0].Name = name
	deployment.Spec.Template.Spec.Containers[0].Image = workload.Image
	deployment.Spec.Template.Spec.Containers[0].ImagePullPolicy = policy
	deployment.Spec.Template.Spec.Containers[0].EnvFrom = envFrom
	deployment.Spec.Template.Spec.Containers[0].VolumeMounts = volumeMounts
	deployment.Spec.Template.Spec.Containers[0].Resources = resources
	deployment.Spec.Template.Spec.ImagePullSecrets = imagePullSecrets
	deployment.Spec.Template.Spec.Volumes = volumes
	if err := applyDeploymentOverrides(deployment, workload.Overrides.Raw); err != nil {
		return err
	}
	deployment.Spec.Template.Spec.Containers[0].Ports = []corev1.ContainerPort{
		{
			ContainerPort: workload.ContainerPort,
			Protocol:      corev1.ProtocolTCP,
		},
	}

	return r.Update(ctx, deployment)
}

func (r *AppDeployReconciler) reconcileService(ctx context.Context, namespace string, workload *appdeployv1alpha1.AppDeployWorkload) error {
	if workload.ServiceType == "" {
		return nil
	}

	servicePort := workload.ContainerPort
	if workload.ServicePort != nil {
		servicePort = *workload.ServicePort
	}

	service := &corev1.Service{}
	key := client.ObjectKey{Name: workload.Name, Namespace: namespace}
	err := r.Get(ctx, key, service)
	if apierrors.IsNotFound(err) {
		service = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      workload.Name,
				Namespace: namespace,
			},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceType(workload.ServiceType),
				Selector: map[string]string{
					"appdeploy.appdeploy.io/workload": workload.Name,
				},
				Ports: []corev1.ServicePort{
					{
						Port:       servicePort,
						TargetPort: intstr.FromInt(int(workload.ContainerPort)),
						Protocol:   corev1.ProtocolTCP,
					},
				},
			},
		}
		return r.Create(ctx, service)
	}
	if err != nil {
		return err
	}

	service.Spec.Type = corev1.ServiceType(workload.ServiceType)
	service.Spec.Selector = map[string]string{
		"appdeploy.appdeploy.io/workload": workload.Name,
	}
	service.Spec.Ports = []corev1.ServicePort{
		{
			Port:       servicePort,
			TargetPort: intstr.FromInt(int(workload.ContainerPort)),
			Protocol:   corev1.ProtocolTCP,
		},
	}

	return r.Update(ctx, service)
}

func (r *AppDeployReconciler) reconcileStatefulSet(ctx context.Context, namespace string, workload *appdeployv1alpha1.AppDeployWorkload) error {
	name := workload.Name
	replicas := int32(1)
	if workload.Replicas != nil {
		replicas = *workload.Replicas
	}
	envFrom := buildEnvFromSources(workload)
	imagePullSecrets := buildImagePullSecrets(workload)
	policy := imagePullPolicy(workload)
	volumeMounts := buildVolumeMounts(workload)
	volumes := buildVolumes(workload)
	resources := workload.Resources

	serviceName := workload.HeadlessServiceName
	if serviceName == "" {
		serviceName = name
	}

	if err := r.reconcileHeadlessService(ctx, namespace, serviceName, name, workload.ContainerPort); err != nil {
		return err
	}

	statefulSet := &appsv1.StatefulSet{}
	key := client.ObjectKey{Name: name, Namespace: namespace}
	err := r.Get(ctx, key, statefulSet)
	if apierrors.IsNotFound(err) {
		statefulSet = &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: appsv1.StatefulSetSpec{
				ServiceName: serviceName,
				Replicas:    &replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"appdeploy.appdeploy.io/workload": name,
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"appdeploy.appdeploy.io/workload": name,
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:            name,
								Image:           workload.Image,
								ImagePullPolicy: policy,
								EnvFrom:         envFrom,
								VolumeMounts:    volumeMounts,
								Resources:       resources,
								Ports: []corev1.ContainerPort{
									{
										ContainerPort: workload.ContainerPort,
										Protocol:      corev1.ProtocolTCP,
									},
								},
							},
						},
						ImagePullSecrets: imagePullSecrets,
						Volumes:          volumes,
					},
				},
			},
		}
		return r.Create(ctx, statefulSet)
	}
	if err != nil {
		return err
	}

	statefulSet.Spec.ServiceName = serviceName
	statefulSet.Spec.Replicas = &replicas
	statefulSet.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"appdeploy.appdeploy.io/workload": name,
		},
	}
	statefulSet.Spec.Template.Labels = map[string]string{
		"appdeploy.appdeploy.io/workload": name,
	}
	if len(statefulSet.Spec.Template.Spec.Containers) == 0 {
		statefulSet.Spec.Template.Spec.Containers = []corev1.Container{{Name: name}}
	}
	statefulSet.Spec.Template.Spec.Containers[0].Name = name
	statefulSet.Spec.Template.Spec.Containers[0].Image = workload.Image
	statefulSet.Spec.Template.Spec.Containers[0].ImagePullPolicy = policy
	statefulSet.Spec.Template.Spec.Containers[0].EnvFrom = envFrom
	statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts = volumeMounts
	statefulSet.Spec.Template.Spec.Containers[0].Resources = resources
	statefulSet.Spec.Template.Spec.ImagePullSecrets = imagePullSecrets
	statefulSet.Spec.Template.Spec.Volumes = volumes
	statefulSet.Spec.Template.Spec.Containers[0].Ports = []corev1.ContainerPort{
		{
			ContainerPort: workload.ContainerPort,
			Protocol:      corev1.ProtocolTCP,
		},
	}

	return r.Update(ctx, statefulSet)
}

func (r *AppDeployReconciler) reconcileHeadlessService(ctx context.Context, namespace, serviceName, workloadName string, containerPort int32) error {
	service := &corev1.Service{}
	key := client.ObjectKey{Name: serviceName, Namespace: namespace}
	err := r.Get(ctx, key, service)
	if apierrors.IsNotFound(err) {
		service = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: namespace,
			},
			Spec: corev1.ServiceSpec{
				ClusterIP: corev1.ClusterIPNone,
				Selector: map[string]string{
					"appdeploy.appdeploy.io/workload": workloadName,
				},
				Ports: []corev1.ServicePort{
					{
						Port:       containerPort,
						TargetPort: intstr.FromInt(int(containerPort)),
						Protocol:   corev1.ProtocolTCP,
					},
				},
			},
		}
		return r.Create(ctx, service)
	}
	if err != nil {
		return err
	}

	service.Spec.ClusterIP = corev1.ClusterIPNone
	service.Spec.Selector = map[string]string{
		"appdeploy.appdeploy.io/workload": workloadName,
	}
	service.Spec.Ports = []corev1.ServicePort{
		{
			Port:       containerPort,
			TargetPort: intstr.FromInt(int(containerPort)),
			Protocol:   corev1.ProtocolTCP,
		},
	}

	return r.Update(ctx, service)
}

func (r *AppDeployReconciler) reconcileIngress(ctx context.Context, namespace string, ingress *appdeployv1alpha1.AppDeployIngress) error {
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

func buildIngressRules(ingress *appdeployv1alpha1.AppDeployIngress) []networkingv1.IngressRule {
	rules := make([]networkingv1.IngressRule, 0, len(ingress.Rules))
	httpPaths := make([]networkingv1.HTTPIngressPath, 0, len(ingress.Rules))
	for _, rule := range ingress.Rules {
		httpPaths = append(httpPaths, networkingv1.HTTPIngressPath{
			Path:     rule.Path,
			PathType: pathTypePtr(networkingv1.PathTypePrefix),
			Backend: networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{
					Name: rule.ServiceName,
					Port: networkingv1.ServiceBackendPort{
						Name:   "",
						Number: 0,
						// set below
					},
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

//go:fix inline
func pathTypePtr(pathType networkingv1.PathType) *networkingv1.PathType {
	return new(pathType)
}

func buildEnvFromSources(workload *appdeployv1alpha1.AppDeployWorkload) []corev1.EnvFromSource {
	envFrom := make([]corev1.EnvFromSource, 0, len(workload.EnvFromConfig)+len(workload.EnvFromSecrets))
	for _, configMapName := range workload.EnvFromConfig {
		envFrom = append(envFrom, corev1.EnvFromSource{
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMapName,
				},
			},
		})
	}
	for _, secretName := range workload.EnvFromSecrets {
		envFrom = append(envFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
			},
		})
	}
	return envFrom
}

func imagePullPolicy(workload *appdeployv1alpha1.AppDeployWorkload) corev1.PullPolicy {
	if workload.ImagePullPolicy != "" {
		return corev1.PullPolicy(workload.ImagePullPolicy)
	}
	return corev1.PullIfNotPresent
}

func buildImagePullSecrets(workload *appdeployv1alpha1.AppDeployWorkload) []corev1.LocalObjectReference {
	if len(workload.ImagePullSecrets) == 0 {
		return nil
	}

	imagePullSecrets := make([]corev1.LocalObjectReference, 0, len(workload.ImagePullSecrets))
	for _, name := range workload.ImagePullSecrets {
		imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReference{Name: name})
	}
	return imagePullSecrets
}

func buildVolumeMounts(workload *appdeployv1alpha1.AppDeployWorkload) []corev1.VolumeMount {
	if len(workload.VolumeMounts) == 0 {
		return nil
	}

	volumeMounts := make([]corev1.VolumeMount, 0, len(workload.VolumeMounts))
	for _, mount := range workload.VolumeMounts {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      mount.Name,
			MountPath: mount.MountPath,
		})
	}
	return volumeMounts
}

func buildVolumes(workload *appdeployv1alpha1.AppDeployWorkload) []corev1.Volume {
	if len(workload.VolumeMounts) == 0 {
		return nil
	}

	volumes := make([]corev1.Volume, 0, len(workload.VolumeMounts))
	for _, mount := range workload.VolumeMounts {
		volume := corev1.Volume{
			Name: mount.Name,
		}
		if mount.ConfigMapName != "" {
			volume.VolumeSource.ConfigMap = &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: mount.ConfigMapName},
			}
		}
		if mount.SecretName != "" {
			volume.VolumeSource.Secret = &corev1.SecretVolumeSource{
				SecretName: mount.SecretName,
			}
		}
		volumes = append(volumes, volume)
	}
	return volumes
}

func applyDeploymentOverrides(deployment *appsv1.Deployment, raw []byte) error {
	if len(raw) == 0 {
		return nil
	}

	var overrides map[string]json.RawMessage
	if err := json.Unmarshal(raw, &overrides); err != nil {
		return fmt.Errorf("invalid deployment overrides: %w", err)
	}

	for key := range overrides {
		if _, ok := deploymentOverrideAllowlist[key]; !ok {
			return fmt.Errorf("deployment override path %q is not allowed", key)
		}
	}

	for key, value := range overrides {
		switch key {
		case "metadata.annotations":
			if err := json.Unmarshal(value, &deployment.Annotations); err != nil {
				return fmt.Errorf("invalid deployment override %q: %w", key, err)
			}
		case "metadata.labels":
			if err := json.Unmarshal(value, &deployment.Labels); err != nil {
				return fmt.Errorf("invalid deployment override %q: %w", key, err)
			}
		case "spec.strategy":
			if err := json.Unmarshal(value, &deployment.Spec.Strategy); err != nil {
				return fmt.Errorf("invalid deployment override %q: %w", key, err)
			}
		case "spec.template.metadata.annotations":
			if err := json.Unmarshal(value, &deployment.Spec.Template.Annotations); err != nil {
				return fmt.Errorf("invalid deployment override %q: %w", key, err)
			}
		case "spec.template.spec.securityContext":
			var securityContext corev1.PodSecurityContext
			if err := json.Unmarshal(value, &securityContext); err != nil {
				return fmt.Errorf("invalid deployment override %q: %w", key, err)
			}
			deployment.Spec.Template.Spec.SecurityContext = &securityContext
		}
	}

	return nil
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

// SetupWithManager sets up the controller with the Manager.
func (r *AppDeployReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appdeployv1alpha1.AppDeploy{}).
		Named("appdeploy").
		Complete(r)
}
