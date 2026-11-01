package render

import (
	"bytes"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

func YAML(objects []client.Object) ([]byte, error) {
	scheme, err := yamlScheme()
	if err != nil {
		return nil, err
	}

	documents := make([]string, 0, len(objects))
	for _, object := range objects {
		data, err := encodeObject(scheme, object)
		if err != nil {
			return nil, err
		}
		documents = append(documents, string(data))
	}

	return []byte(strings.Join(documents, "\n---\n") + "\n"), nil
}

func yamlScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{
		corev1.AddToScheme,
		appsv1.AddToScheme,
		batchv1.AddToScheme,
		networkingv1.AddToScheme,
	} {
		if err := addToScheme(scheme); err != nil {
			return nil, err
		}
	}
	return scheme, nil
}

func encodeObject(scheme *runtime.Scheme, object client.Object) ([]byte, error) {
	if _, ok := object.(*unstructured.Unstructured); !ok {
		gvks, _, err := scheme.ObjectKinds(object)
		if err != nil {
			return nil, fmt.Errorf("find resource type for %T: %w", object, err)
		}
		if len(gvks) != 1 {
			return nil, fmt.Errorf("expected one resource type for %T", object)
		}
		object.GetObjectKind().SetGroupVersionKind(gvks[0])
	}

	data, err := yaml.Marshal(object)
	if err != nil {
		return nil, fmt.Errorf("encode %T: %w", object, err)
	}
	return bytes.TrimSpace(data), nil
}
