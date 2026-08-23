package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/ude-p/appdeploy/internal/config"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func BuildManifests(model config.Model) ([]client.Object, error) {
	objects := make([]client.Object, 0, len(model.Manifests))
	for _, manifest := range model.Manifests {
		decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest.YAML), 4096)
		found := false
		for {
			var document map[string]any
			if err := decoder.Decode(&document); err != nil {
				if err == io.EOF {
					break
				}
				return nil, fmt.Errorf("manifest %q: decode yaml: %w", manifest.Name, err)
			}
			if len(document) == 0 {
				continue
			}
			found = true

			apiVersion, _ := document["apiVersion"].(string)
			kind, _ := document["kind"].(string)
			if apiVersion == "" || kind == "" {
				return nil, fmt.Errorf("manifest %q: yaml must include apiVersion and kind", manifest.Name)
			}

			object := &unstructured.Unstructured{Object: document}
			object.SetGroupVersionKind(schema.FromAPIVersionAndKind(apiVersion, kind))
			objects = append(objects, object)
		}
		if !found {
			return nil, fmt.Errorf("manifest %q: yaml is required", manifest.Name)
		}
	}
	return objects, nil
}
