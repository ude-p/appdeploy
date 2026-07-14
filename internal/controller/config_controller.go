package controller

import (
	"context"
	"fmt"
	"maps"
	"sort"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appdeployv1 "github.com/ude-p/appdeploy/api/v1"
)

func (r *AppDeployReconciler) reconcileNamespaces(ctx context.Context, namespaces []string) error {
	for _, name := range namespaces {
		namespace := &corev1.Namespace{}
		key := client.ObjectKey{Name: name}
		err := r.Get(ctx, key, namespace)
		if apierrors.IsNotFound(err) {
			namespace = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
			}
			if createErr := r.Create(ctx, namespace); createErr != nil {
				return createErr
			}
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func validateConfigMaps(configMaps []appdeployv1.AppDeployConfigMap, namespaceSet map[string]struct{}) error {
	defaultConfigMaps := make(map[string]appdeployv1.AppDeployConfigMap)
	scopedConfigMaps := make(map[string]int)
	for i, configMap := range configMaps {
		if configMap.Scope != "" {
			if _, ok := namespaceSet[configMap.Scope]; !ok {
				return fmt.Errorf("spec.configMaps[%d].scope %q must match one of spec.namespaces", i, configMap.Scope)
			}
		}
		if configMap.Override && configMap.Scope == "" {
			return fmt.Errorf("spec.configMaps[%d].override requires scope", i)
		}
		if configMap.Scope == "" {
			if _, ok := defaultConfigMaps[configMap.Name]; ok {
				return fmt.Errorf("spec.configMaps[%d].name %q duplicates default config map", i, configMap.Name)
			}
			defaultConfigMaps[configMap.Name] = configMap
			continue
		}
		key := configMap.Scope + "/" + configMap.Name
		if previousIndex, ok := scopedConfigMaps[key]; ok {
			return fmt.Errorf("spec.configMaps[%d] duplicates spec.configMaps[%d] for scope %q and name %q", i, previousIndex, configMap.Scope, configMap.Name)
		}
		scopedConfigMaps[key] = i
	}
	for i, configMap := range configMaps {
		if configMap.Scope != "" && !configMap.Override {
			if _, ok := defaultConfigMaps[configMap.Name]; ok {
				return fmt.Errorf("spec.configMaps[%d].name %q duplicates default config map; set override true", i, configMap.Name)
			}
		}
		if !configMap.Override {
			continue
		}
		defaultConfigMap, ok := defaultConfigMaps[configMap.Name]
		if !ok {
			return fmt.Errorf("spec.configMaps[%d].override requires default config map %q", i, configMap.Name)
		}
		for key := range configMap.Data {
			if _, ok := defaultConfigMap.Data[key]; !ok {
				return fmt.Errorf("spec.configMaps[%d].data[%q] cannot override missing default key", i, key)
			}
		}
	}
	return nil
}

func validatePersistentVolumeClaims(pvcs []appdeployv1.AppDeployPersistentVolumeClaim, namespaceSet map[string]struct{}) error {
	targets := make(map[string]int)
	defaults := make(map[string]int)
	scoped := make(map[string]int)
	for i, pvc := range pvcs {
		if pvc.Scope != "" {
			if _, ok := namespaceSet[pvc.Scope]; !ok {
				return fmt.Errorf("spec.persistentVolumeClaims[%d].scope %q must match one of spec.namespaces", i, pvc.Scope)
			}
		}
		if len(pvc.AccessModes) == 0 {
			return fmt.Errorf("spec.persistentVolumeClaims[%d].accessModes must not be empty", i)
		}
		if storage, ok := pvc.Resources.Requests[corev1.ResourceStorage]; !ok || storage.IsZero() {
			return fmt.Errorf("spec.persistentVolumeClaims[%d].resources.requests.storage must be set", i)
		}

		key := pvc.Scope + "/" + pvc.Name
		if previousIndex, ok := targets[key]; ok {
			return fmt.Errorf("spec.persistentVolumeClaims[%d] duplicates spec.persistentVolumeClaims[%d] for scope %q and name %q", i, previousIndex, pvc.Scope, pvc.Name)
		}
		if pvc.Scope == "" {
			if previousIndex, ok := scoped[pvc.Name]; ok {
				return fmt.Errorf("spec.persistentVolumeClaims[%d].name %q duplicates scoped persistent volume claim spec.persistentVolumeClaims[%d]", i, pvc.Name, previousIndex)
			}
			defaults[pvc.Name] = i
		} else if previousIndex, ok := defaults[pvc.Name]; ok {
			return fmt.Errorf("spec.persistentVolumeClaims[%d].name %q duplicates default persistent volume claim spec.persistentVolumeClaims[%d]", i, pvc.Name, previousIndex)
		} else {
			scoped[pvc.Name] = i
		}
		targets[key] = i
	}
	return nil
}

func (r *AppDeployReconciler) ensureESOConfigured() error {
	if r.RESTMapper == nil {
		return fmt.Errorf("external secrets operator is not configured: rest mapper is unavailable")
	}

	_, err := r.RESTMapper.RESTMapping(schema.GroupKind{Group: "external-secrets.io", Kind: "ExternalSecret"}, "v1")
	if err != nil {
		return fmt.Errorf("external secrets operator is not configured: %w", err)
	}

	return nil
}

func (r *AppDeployReconciler) reconcileConfigMaps(ctx context.Context, namespace string, configMaps []appdeployv1.AppDeployConfigMap) error {
	desired := map[string]appdeployv1.AppDeployConfigMap{}
	for i := range configMaps {
		configMap := configMaps[i]
		if configMap.Scope != "" {
			continue
		}
		configMap.Data = copyStringMap(configMap.Data)
		desired[configMap.Name] = configMap
	}
	for i := range configMaps {
		configMap := configMaps[i]
		if configMap.Scope != namespace {
			continue
		}
		if !configMap.Override {
			configMap.Data = copyStringMap(configMap.Data)
			desired[configMap.Name] = configMap
			continue
		}
		base := desired[configMap.Name]
		base.Data = copyStringMap(base.Data)
		maps.Copy(base.Data, configMap.Data)
		desired[configMap.Name] = base
	}

	names := make([]string, 0, len(desired))
	for name := range desired {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		configMap := desired[name]
		if err := r.reconcileConfigMap(ctx, namespace, &configMap); err != nil {
			return err
		}
	}
	return nil
}

func (r *AppDeployReconciler) reconcileConfigMap(ctx context.Context, namespace string, configMap *appdeployv1.AppDeployConfigMap) error {
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

func (r *AppDeployReconciler) reconcilePersistentVolumeClaim(ctx context.Context, namespace string, pvc *appdeployv1.AppDeployPersistentVolumeClaim) error {
	claim := &corev1.PersistentVolumeClaim{}
	key := client.ObjectKey{Name: pvc.Name, Namespace: namespace}
	err := r.Get(ctx, key, claim)
	if apierrors.IsNotFound(err) {
		claim = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvc.Name,
				Namespace: namespace,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: pvc.AccessModes,
				Resources:   pvc.Resources,
			},
		}
		if pvc.StorageClassName != "" {
			claim.Spec.StorageClassName = &pvc.StorageClassName
		}
		return r.Create(ctx, claim)
	}
	if err != nil {
		return err
	}

	claim.Spec.Resources = pvc.Resources
	return r.Update(ctx, claim)
}

func copyStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	copied := make(map[string]string, len(values))
	maps.Copy(copied, values)
	return copied
}

func (r *AppDeployReconciler) reconcileExternalSecret(ctx context.Context, namespace string, secret *appdeployv1.AppDeploySecret) error {
	gvk := schema.GroupVersionKind{
		Group:   "external-secrets.io",
		Version: "v1",
		Kind:    "ExternalSecret",
	}

	targetType := "Opaque"
	if secret.Type == "ImagePull" {
		targetType = "kubernetes.io/dockerconfigjson"
	}

	externalSecret := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "external-secrets.io/v1",
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
