package render

import (
	"fmt"

	"github.com/ude-p/appdeploy/internal/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Build(model config.Model, namespace string) ([]client.Object, error) {
	if !model.HasNamespace(namespace) {
		return nil, fmt.Errorf("namespace %q is not declared", namespace)
	}

	objects := []client.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}},
	}

	for _, builder := range []func(config.Model, string) ([]client.Object, error){
		buildConfigMaps,
		buildSecrets,
		buildPersistentVolumeClaims,
		buildApps,
		buildStatefulSets,
		buildJobs,
		buildCronJobs,
		buildRouteMiddlewares,
		buildIngresses,
	} {
		resources, err := builder(model, namespace)
		if err != nil {
			return nil, err
		}
		objects = append(objects, resources...)
	}

	return objects, nil
}
