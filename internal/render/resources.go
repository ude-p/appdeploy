package render

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/joho/godotenv"
	"github.com/ude-p/appdeploy/internal/config"
	"github.com/zclconf/go-cty/cty"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func buildConfigMaps(model config.Model, namespace string) ([]client.Object, error) {
	objects := make([]client.Object, 0, len(model.Configs))
	for _, item := range model.Configs {
		data, err := configData(item.Values)
		if err != nil {
			return nil, fmt.Errorf("config %q: %w", item.Name, err)
		}
		if err := mergeEnvSource(model, item.Env, data); err != nil {
			return nil, fmt.Errorf("config %q: %w", item.Name, err)
		}
		maps.Copy(data, item.Data)
		for _, file := range item.Files {
			contents, err := os.ReadFile(sourcePath(model, file.Source))
			if err != nil {
				return nil, fmt.Errorf("config %q file %q: %w", item.Name, file.Name, err)
			}
			data[file.Name] = string(contents)
		}
		if item.Scope != nil && item.Scope.Namespace == namespace {
			if err := mergeEnvSource(model, item.Scope.Env, data); err != nil {
				return nil, fmt.Errorf("config %q scope %q: %w", item.Name, namespace, err)
			}
			overrides, err := configData(item.Scope.Values)
			if err != nil {
				return nil, fmt.Errorf("config %q scope %q: %w", item.Name, namespace, err)
			}
			maps.Copy(data, overrides)
			maps.Copy(data, item.Scope.Data)
			for _, file := range item.Scope.Files {
				contents, err := os.ReadFile(sourcePath(model, file.Source))
				if err != nil {
					return nil, fmt.Errorf("config %q scope %q file %q: %w", item.Name, namespace, file.Name, err)
				}
				data[file.Name] = string(contents)
			}
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
		data := make(map[string]string)
		if err := mergeEnvSource(model, item.Env, data); err != nil {
			return nil, fmt.Errorf("secret %q: %w", item.Name, err)
		}
		if err := mergeSecretFiles(model, item.Files, data); err != nil {
			return nil, fmt.Errorf("secret %q: %w", item.Name, err)
		}
		if item.Scope != nil && item.Scope.Namespace == namespace {
			if err := mergeEnvSource(model, item.Scope.Env, data); err != nil {
				return nil, fmt.Errorf("secret %q scope %q: %w", item.Name, namespace, err)
			}
			if err := mergeSecretFiles(model, item.Scope.Files, data); err != nil {
				return nil, fmt.Errorf("secret %q scope %q: %w", item.Name, namespace, err)
			}
		}

		objects = append(objects, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: item.Name, Namespace: namespace},
			StringData: data,
		})
	}
	return objects, nil
}

func mergeEnvSource(model config.Model, source *config.EnvSource, data map[string]string) error {
	if source == nil {
		return nil
	}
	values, err := readEnvSource(model, source.Source, source.Prefix)
	if err != nil {
		return err
	}
	maps.Copy(data, values)
	return nil
}

func mergeSecretFiles(model config.Model, files []config.SecretFile, data map[string]string) error {
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

func configData(body hcl.Body) (map[string]string, error) {
	data := make(map[string]string)
	if body == nil {
		return data, nil
	}
	syntaxBody, ok := body.(*hclsyntax.Body)
	if !ok {
		return nil, fmt.Errorf("read config values")
	}
	for name, attribute := range syntaxBody.Attributes {
		if name == "data" {
			continue
		}
		value, diagnostics := attribute.Expr.Value(nil)
		if diagnostics.HasErrors() {
			return nil, fmt.Errorf("%s: %s", name, diagnostics.Error())
		}
		if value.Type() != cty.String {
			return nil, fmt.Errorf("%s must be a string", name)
		}
		data[name] = value.AsString()
	}
	return data, nil
}

func sourcePath(model config.Model, source string) string {
	if filepath.IsAbs(source) {
		return source
	}
	return filepath.Join(filepath.Dir(model.SourcePath), source)
}
