package render

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ude-p/appdeploy/internal/config"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const manifestsDir = "_manifests"

type kustomizationFile struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Resources  []string `yaml:"resources"`
}

func Kustomize(model config.Model, root string, namespaces []string) error {
	scheme, err := yamlScheme()
	if err != nil {
		return err
	}

	groups := make(map[string][]client.Object, len(namespaces)+1)
	for _, namespace := range namespaces {
		objects, err := Build(model, namespace)
		if err != nil {
			return err
		}
		groups[namespace] = objects
	}
	manifests, err := BuildManifests(model)
	if err != nil {
		return err
	}
	if len(manifests) > 0 {
		groups[manifestsDir] = manifests
	}

	if hasSecret(groups) {
		if _, err := exec.LookPath("sops"); err != nil {
			return fmt.Errorf("sops is required to encrypt secrets: %w", err)
		}
	}

	for name, objects := range groups {
		if err := writeKustomization(scheme, root, name, objects); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func writeKustomization(scheme *runtime.Scheme, root, name string, objects []client.Object) error {
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	resources := make([]string, 0, len(objects))
	seen := make(map[string]struct{}, len(objects))
	for _, object := range objects {
		if object.GetName() == "" {
			return fmt.Errorf("%T has no name", object)
		}
		secret, encrypt := object.(*corev1.Secret)
		if encrypt {
			object = secretWithData(secret)
		}
		data, err := encodeObject(scheme, object)
		if err != nil {
			return err
		}
		fileName := strings.ToLower(object.GetObjectKind().GroupVersionKind().Kind) + "-" + object.GetName() + ".yaml"
		if _, exists := seen[fileName]; exists {
			return fmt.Errorf("%s is generated more than once", fileName)
		}
		seen[fileName] = struct{}{}
		if err := writeFile(dir, fileName, append(data, '\n'), encrypt); err != nil {
			return err
		}
		resources = append(resources, fileName)
	}

	sort.Strings(resources)
	var content bytes.Buffer
	encoder := yaml.NewEncoder(&content)
	encoder.SetIndent(2)
	if err := encoder.Encode(kustomizationFile{
		APIVersion: "kustomize.config.k8s.io/v1beta1",
		Kind:       "Kustomization",
		Resources:  resources,
	}); err != nil {
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "kustomization.yaml"), content.Bytes(), 0o644)
}

func writeFile(dir, name string, data []byte, encrypt bool) error {
	path := filepath.Join(dir, name)
	if !encrypt {
		return os.WriteFile(path, data, 0o644)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	cmd := exec.Command("sops", "-e", "-i", "--encrypted-regex", "^(data|stringData)$", path)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("encrypt %s: %w", name, err)
	}
	return nil
}

func secretWithData(secret *corev1.Secret) *corev1.Secret {
	if len(secret.StringData) == 0 {
		return secret
	}
	copy := secret.DeepCopy()
	copy.Data = make(map[string][]byte, len(secret.StringData))
	for name, value := range secret.StringData {
		copy.Data[name] = []byte(value)
	}
	copy.StringData = nil
	return copy
}

func hasSecret(groups map[string][]client.Object) bool {
	for _, objects := range groups {
		for _, object := range objects {
			if _, ok := object.(*corev1.Secret); ok {
				return true
			}
		}
	}
	return false
}
