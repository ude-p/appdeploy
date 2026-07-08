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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type AppDeploySpec struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Namespaces []string `json:"namespaces,omitempty"`
	// +kubebuilder:validation:MinItems=1
	SelectedNamespaces []string             `json:"selectedNamespaces,omitempty"`
	ConfigMaps         []AppDeployConfigMap `json:"configMaps,omitempty"`
	Secrets            []AppDeploySecret    `json:"secrets,omitempty"`
	Workloads          []AppDeployWorkload  `json:"workloads,omitempty"`
	Ingresses          []AppDeployIngress   `json:"ingresses,omitempty"`
}

type AppDeployConfigMap struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +kubebuilder:validation:MinLength=1
	Scope string            `json:"scope,omitempty"`
	Data  map[string]string `json:"data,omitempty"`
}

type AppDeploySecret struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +kubebuilder:default=Opaque
	// +kubebuilder:validation:Enum=Opaque;ImagePull
	Type string `json:"type,omitempty"`
	// +kubebuilder:validation:MinLength=1
	Scope string `json:"scope,omitempty"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	RemoteKey string `json:"remoteKey"`
	// +kubebuilder:default=cluster-vault
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	SecretStoreName string `json:"secretStoreName"`
	// +kubebuilder:default=ClusterSecretStore
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=SecretStore;ClusterSecretStore
	SecretStoreKind string `json:"secretStoreKind"`
}

type AppDeployWorkload struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +kubebuilder:default=Deployment
	// +kubebuilder:validation:Enum=Deployment;StatefulSet;Job
	Kind string `json:"kind,omitempty"`
	// +kubebuilder:validation:MinLength=1
	Scope string `json:"scope,omitempty"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Image   string   `json:"image"`
	Command []string `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	// +kubebuilder:default:=1
	Replicas *int32 `json:"replicas,omitempty"`
	// +kubebuilder:validation:Minimum=0
	BackoffLimit *int32 `json:"backoffLimit,omitempty"`
	// +kubebuilder:validation:Minimum=0
	TTLSecondsAfterFinished *int32                      `json:"ttlSecondsAfterFinished,omitempty"`
	ContainerPort           *int32                      `json:"containerPort,omitempty"`
	Resources               corev1.ResourceRequirements `json:"resources,omitempty"`
	// +kubebuilder:validation:Enum=Always;IfNotPresent;Never
	ImagePullPolicy     string                 `json:"imagePullPolicy,omitempty"`
	ServiceType         string                 `json:"serviceType,omitempty"`
	ServicePort         *int32                 `json:"servicePort,omitempty"`
	HeadlessServiceName string                 `json:"headlessServiceName,omitempty"`
	EnvFromConfig       []string               `json:"envFromConfig,omitempty"`
	EnvFromSecrets      []string               `json:"envFromSecrets,omitempty"`
	ImagePullSecrets    []string               `json:"imagePullSecrets,omitempty"`
	VolumeMounts        []AppDeployVolumeMount `json:"volumeMounts,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Overrides apiextensionsv1.JSON `json:"overrides,omitempty"`
}

type AppDeployVolumeMount struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	MountPath string `json:"mountPath"`
	// +kubebuilder:validation:MinLength=1
	ConfigMapName string `json:"configMapName,omitempty"`
	// +kubebuilder:validation:MinLength=1
	SecretName string `json:"secretName,omitempty"`
}

type AppDeployIngress struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +kubebuilder:validation:MinLength=1
	Scope string `json:"scope,omitempty"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ClassName string `json:"className,omitempty"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Host          string                 `json:"host"`
	Annotations   map[string]string      `json:"annotations,omitempty"`
	TLSSecretName string                 `json:"tlsSecretName,omitempty"`
	Rules         []AppDeployIngressRule `json:"rules,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Overrides apiextensionsv1.JSON `json:"overrides,omitempty"`
}

type AppDeployIngressRule struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ServiceName string `json:"serviceName"`
	// +kubebuilder:validation:Required
	ServicePort intstr.IntOrString `json:"servicePort"`
}

type AppDeployStatus struct {
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

type AppDeploy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitzero"`
	Spec              AppDeploySpec   `json:"spec"`
	Status            AppDeployStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

type AppDeployList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AppDeploy `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &AppDeploy{}, &AppDeployList{})
		return nil
	})
}
