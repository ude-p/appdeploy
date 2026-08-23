package main

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const fieldManager = "appd"

func deploy(objects []client.Object, kubeconfigPath, contextName string) error {
	dynamicClient, mapper, err := buildKubeClients(kubeconfigPath, contextName)
	if err != nil {
		return err
	}
	for _, object := range objects {
		if err := applyObject(context.Background(), dynamicClient, mapper, object); err != nil {
			return err
		}
	}
	return nil
}

func buildKubeClients(kubeconfigPath, contextName string) (dynamic.Interface, meta.RESTMapper, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigPath != "" {
		loadingRules.ExplicitPath = kubeconfigPath
	}

	raw, err := loadingRules.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	if len(raw.Contexts) == 0 {
		return nil, nil, fmt.Errorf("kubeconfig has no contexts")
	}

	selected := raw.CurrentContext
	if contextName != "" {
		if _, exists := raw.Contexts[contextName]; !exists {
			return nil, nil, fmt.Errorf("context %q is not declared in kubeconfig", contextName)
		}
		selected = contextName
	}
	if selected == "" {
		return nil, nil, fmt.Errorf("kubeconfig has no current context")
	}
	if _, exists := raw.Contexts[selected]; !exists {
		return nil, nil, fmt.Errorf("current context %q is not declared in kubeconfig", selected)
	}

	overrides := &clientcmd.ConfigOverrides{CurrentContext: selected}
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("build kubernetes client config: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("build kubernetes dynamic client: %w", err)
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("build kubernetes discovery client: %w", err)
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(discoveryClient))
	return dynamicClient, mapper, nil
}

func applyObject(ctx context.Context, dynamicClient dynamic.Interface, mapper meta.RESTMapper, object client.Object) error {
	key := client.ObjectKeyFromObject(object)
	gvk := object.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		gvks, _, err := clientgoscheme.Scheme.ObjectKinds(object)
		if err != nil {
			return fmt.Errorf("find resource type for %T: %w", object, err)
		}
		if len(gvks) != 1 {
			return fmt.Errorf("expected one resource type for %T", object)
		}
		gvk = gvks[0]
	}

	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return fmt.Errorf("map %T %v: %w", object, key, err)
	}

	var payload []byte
	if unstructuredObject, ok := object.(*unstructured.Unstructured); ok {
		if unstructuredObject.GetAPIVersion() == "" {
			unstructuredObject.SetAPIVersion(gvk.GroupVersion().String())
		}
		if unstructuredObject.GetKind() == "" {
			unstructuredObject.SetKind(gvk.Kind)
		}
		payload, err = json.Marshal(unstructuredObject.Object)
	} else {
		data, convertErr := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
		if convertErr != nil {
			return fmt.Errorf("marshal %T %v: %w", object, key, convertErr)
		}
		data["apiVersion"] = gvk.GroupVersion().String()
		data["kind"] = gvk.Kind
		payload, err = json.Marshal(data)
	}
	if err != nil {
		return fmt.Errorf("marshal %T %v: %w", object, key, err)
	}

	var resource dynamic.ResourceInterface
	switch mapping.Scope.Name() {
	case meta.RESTScopeNameNamespace:
		if object.GetNamespace() == "" {
			return fmt.Errorf("%T %v: namespace is required", object, key)
		}
		resource = dynamicClient.Resource(mapping.Resource).Namespace(object.GetNamespace())
	default:
		if object.GetNamespace() != "" {
			return fmt.Errorf("%T %v: namespace must be empty for cluster-scoped resources", object, key)
		}
		resource = dynamicClient.Resource(mapping.Resource)
	}

	_, err = resource.Patch(ctx, object.GetName(), types.ApplyPatchType, payload, metav1.PatchOptions{
		FieldManager: fieldManager,
		Force:        new(bool),
	})
	if err != nil {
		return fmt.Errorf("apply %T %v: %w", object, key, err)
	}
	return nil
}
