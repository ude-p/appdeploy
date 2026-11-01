package render

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/joho/godotenv"
	"github.com/ude-p/appdeploy/internal/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func buildConfigMaps(model config.Model, namespace string) ([]client.Object, error) {
	objects := make([]client.Object, 0, len(model.Configs))
	for _, item := range model.Configs {
		data, err := buildValues(model, namespace, item.Source, item.Prefix, item.Files, item.Scopes, item.Values, true)
		if err != nil {
			return nil, fmt.Errorf("config %q: %w", item.Name, err)
		}
		objects = append(objects, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: item.Name, Namespace: namespace},
			Data:       data,
		})
	}
	return objects, nil
}

func buildSecrets(model config.Model, namespace string) ([]client.Object, error) {
	objects := make([]client.Object, 0, len(model.Secrets))
	for _, item := range model.Secrets {
		data, err := buildValues(model, namespace, item.Source, item.Prefix, item.Files, item.Scopes, nil, false)
		if err != nil {
			return nil, fmt.Errorf("secret %q: %w", item.Name, err)
		}
		objects = append(objects, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: item.Name, Namespace: namespace},
			StringData: data,
		})
	}
	return objects, nil
}

func inlineResources(model config.Model, namespace, name string, env *config.Env) ([]client.Object, error) {
	if env == nil {
		return nil, nil
	}
	var objects []client.Object
	if env.Config != nil {
		data, err := buildValues(model, namespace, env.Config.Source, env.Config.Prefix, env.Config.Files, env.Config.Scopes, env.Config.Values, true)
		if err != nil {
			return nil, fmt.Errorf("config: %w", err)
		}
		objects = append(objects, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: name + "-config", Namespace: namespace},
			Data:       data,
		})
	}
	if env.Secret != nil {
		data, err := buildValues(model, namespace, env.Secret.Source, env.Secret.Prefix, env.Secret.Files, env.Secret.Scopes, nil, false)
		if err != nil {
			return nil, fmt.Errorf("secret: %w", err)
		}
		objects = append(objects, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name + "-secret", Namespace: namespace},
			StringData: data,
		})
	}
	return objects, nil
}

func buildValues(model config.Model, namespace, source, prefix string, files []config.File, scopes []config.Scope, body hcl.Body, literals bool) (map[string]string, error) {
	data := make(map[string]string)
	if literals {
		values, err := config.StringValues(body, "source", "prefix")
		if err != nil {
			return nil, err
		}
		maps.Copy(data, values)
	}
	if err := mergeEnvSource(model, source, prefix, data); err != nil {
		return nil, err
	}
	if err := mergeFiles(model, files, data); err != nil {
		return nil, err
	}
	for _, scope := range scopes {
		if scope.Namespace != namespace {
			continue
		}
		if err := mergeEnvSource(model, scope.Source, scope.Prefix, data); err != nil {
			return nil, fmt.Errorf("scope %q: %w", scope.Namespace, err)
		}
		if literals {
			values, err := config.StringValues(scope.Values, "source", "prefix")
			if err != nil {
				return nil, fmt.Errorf("scope %q: %w", scope.Namespace, err)
			}
			maps.Copy(data, values)
		}
		if err := mergeFiles(model, scope.Files, data); err != nil {
			return nil, fmt.Errorf("scope %q: %w", scope.Namespace, err)
		}
	}
	return data, nil
}

func mergeEnvSource(model config.Model, source, prefix string, data map[string]string) error {
	if source == "" {
		return nil
	}
	values, err := readEnvSource(model, source, prefix)
	if err != nil {
		return err
	}
	maps.Copy(data, values)
	return nil
}

func mergeFiles(model config.Model, files []config.File, data map[string]string) error {
	for _, file := range files {
		contents, err := os.ReadFile(sourcePath(model, file.Source))
		if err != nil {
			return fmt.Errorf("file %q: %w", file.Name, err)
		}
		data[file.Name] = string(contents)
	}
	return nil
}

func readEnvSource(model config.Model, source, prefix string) (map[string]string, error) {
	if source == "" {
		return map[string]string{}, nil
	}
	data, err := godotenv.Read(sourcePath(model, source))
	if err != nil {
		return nil, err
	}
	if prefix == "" {
		return data, nil
	}

	filtered := make(map[string]string)
	for name, value := range data {
		if len(name) < len(prefix) || name[:len(prefix)] != prefix {
			continue
		}
		filtered[name[len(prefix):]] = value
	}
	return filtered, nil
}

func buildPersistentVolumeClaims(model config.Model, namespace string) ([]client.Object, error) {
	objects := make([]client.Object, 0, len(model.Volumes))
	for _, volume := range model.Volumes {
		size, err := resource.ParseQuantity(volume.Size)
		if err != nil {
			return nil, fmt.Errorf("volume %q: parse size: %w", volume.Name, err)
		}

		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: volume.Name, Namespace: namespace},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.PersistentVolumeAccessMode(volume.Access)},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: size},
				},
			},
		}
		if volume.StorageClass != "" {
			pvc.Spec.StorageClassName = &volume.StorageClass
		}
		objects = append(objects, pvc)
	}
	return objects, nil
}

func sourcePath(model config.Model, source string) string {
	if filepath.IsAbs(source) {
		return source
	}
	return filepath.Join(filepath.Dir(model.SourcePath), source)
}
