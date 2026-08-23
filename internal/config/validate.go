package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

func (model Model) Validate() error {
	namespaces := make(map[string]struct{}, len(model.Namespaces))
	for _, namespace := range model.Namespaces {
		if err := addName(namespaces, "namespace", namespace.Name); err != nil {
			return err
		}
	}
	if len(namespaces) == 0 {
		return fmt.Errorf("at least one namespace is required")
	}

	configs := make(map[string]struct{}, len(model.Configs))
	for _, config := range model.Configs {
		if err := addName(configs, "config", config.Name); err != nil {
			return err
		}
		if err := validateConfigValues(config.Values); err != nil {
			return fmt.Errorf("config %q: %w", config.Name, err)
		}
		for _, file := range config.Files {
			if err := model.validateSourceFile(file.Source); err != nil {
				return fmt.Errorf("config %q file %q: %w", config.Name, file.Name, err)
			}
		}
		if config.Scope != nil {
			if err := validateHCLScope(config.Scope.Namespace, namespaces); err != nil {
				return fmt.Errorf("config %q: %w", config.Name, err)
			}
			if err := validateConfigValues(config.Scope.Values); err != nil {
				return fmt.Errorf("config %q scope %q: %w", config.Name, config.Scope.Namespace, err)
			}
		}
	}

	secrets := make(map[string]struct{}, len(model.Secrets))
	for _, secret := range model.Secrets {
		if err := addName(secrets, "secret", secret.Name); err != nil {
			return err
		}
		if err := model.validateSecretSource(secret.Source); err != nil {
			return fmt.Errorf("secret %q: %w", secret.Name, err)
		}
		if secret.Scope != nil {
			if err := validateHCLScope(secret.Scope.Namespace, namespaces); err != nil {
				return fmt.Errorf("secret %q: %w", secret.Name, err)
			}
			if err := model.validateSecretSource(secret.Scope.Source); err != nil {
				return fmt.Errorf("secret %q scope %q: %w", secret.Name, secret.Scope.Namespace, err)
			}
		}
	}

	volumes := make(map[string]struct{}, len(model.Volumes))
	for _, volume := range model.Volumes {
		if err := addName(volumes, "volume", volume.Name); err != nil {
			return err
		}
	}

	apps := make(map[string]struct{}, len(model.Apps))
	for _, app := range model.Apps {
		if err := addName(apps, "app", app.Name); err != nil {
			return err
		}
		if err := validateEnv(app.Env, configs, secrets); err != nil {
			return fmt.Errorf("app %q: %w", app.Name, err)
		}
		for _, mount := range app.Mounts {
			if err := validateReference(mount.Volume, "volume", volumes); err != nil {
				return fmt.Errorf("app %q mount: %w", app.Name, err)
			}
		}
	}

	statefuls := make(map[string]struct{}, len(model.Statefuls))
	for _, stateful := range model.Statefuls {
		if err := addName(statefuls, "stateful", stateful.Name); err != nil {
			return err
		}
		if err := validateEnv(stateful.Env, configs, secrets); err != nil {
			return fmt.Errorf("stateful %q: %w", stateful.Name, err)
		}
	}

	if err := validateJobs(model.Jobs, configs, secrets); err != nil {
		return err
	}
	if err := validateCrons(model.Crons, configs, secrets); err != nil {
		return err
	}

	routes := make(map[string]struct{}, len(model.Routes))
	for _, route := range model.Routes {
		if err := addName(routes, "route", route.Name); err != nil {
			return err
		}
		if err := validateReference(route.To, "app", apps); err != nil {
			return fmt.Errorf("route %q: %w", route.Name, err)
		}
	}

	manifests := make(map[string]struct{}, len(model.Manifests))
	for _, manifest := range model.Manifests {
		if err := addName(manifests, "manifest", manifest.Name); err != nil {
			return err
		}
		if err := validateManifestYAML(manifest.YAML); err != nil {
			return fmt.Errorf("manifest %q: %w", manifest.Name, err)
		}
	}

	return nil
}

func validateConfigValues(body hcl.Body) error {
	if body == nil {
		return nil
	}
	_, diagnostics := body.JustAttributes()
	if diagnostics.HasErrors() {
		return fmt.Errorf("only environment variable attributes are allowed")
	}
	return nil
}

func validateJobs(jobs []Job, configs, secrets map[string]struct{}) error {
	names := make(map[string]struct{}, len(jobs))
	for _, job := range jobs {
		if err := addName(names, "job", job.Name); err != nil {
			return err
		}
		if err := validateEnv(job.Env, configs, secrets); err != nil {
			return fmt.Errorf("job %q: %w", job.Name, err)
		}
	}
	return nil
}

func validateCrons(crons []Cron, configs, secrets map[string]struct{}) error {
	names := make(map[string]struct{}, len(crons))
	for _, cron := range crons {
		if err := addName(names, "cron", cron.Name); err != nil {
			return err
		}
		if err := validateEnv(cron.Env, configs, secrets); err != nil {
			return fmt.Errorf("cron %q: %w", cron.Name, err)
		}
	}
	return nil
}

func validateEnv(env *Env, configs, secrets map[string]struct{}) error {
	if env == nil {
		return nil
	}
	if err := validateReference(env.Config, "config", configs); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if err := validateReference(env.Secret, "secret", secrets); err != nil {
		return fmt.Errorf("secret: %w", err)
	}
	return nil
}

func validateHCLScope(namespace string, namespaces map[string]struct{}) error {
	if _, exists := namespaces[namespace]; !exists {
		return fmt.Errorf("scope references unknown namespace %q", namespace)
	}
	return nil
}

func validateReference(expression hcl.Expression, kind string, names map[string]struct{}) error {
	if expression == nil {
		return nil
	}
	traversal, diagnostics := hcl.AbsTraversalForExpr(expression)
	if diagnostics.HasErrors() || len(traversal) != 2 {
		return fmt.Errorf("must be a %s reference", kind)
	}

	root, rootOK := traversal[0].(hcl.TraverseRoot)
	name, nameOK := traversal[1].(hcl.TraverseAttr)
	if !rootOK || !nameOK || root.Name != kind {
		return fmt.Errorf("must be a %s reference", kind)
	}
	if _, exists := names[name.Name]; !exists {
		return fmt.Errorf("references unknown %s %q", kind, name.Name)
	}
	return nil
}

func addName(names map[string]struct{}, kind, name string) error {
	if name == "" {
		return fmt.Errorf("%s name is required", kind)
	}
	if _, exists := names[name]; exists {
		return fmt.Errorf("%s %q is declared more than once", kind, name)
	}
	names[name] = struct{}{}
	return nil
}

func validateManifestYAML(data string) error {
	decoder := yaml.NewDecoder(strings.NewReader(data))
	found := false
	for {
		var document map[string]any
		if err := decoder.Decode(&document); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("read yaml: %w", err)
		}
		if len(document) == 0 {
			continue
		}
		found = true
		kind, _ := document["kind"].(string)
		apiVersion, _ := document["apiVersion"].(string)
		if kind == "" || apiVersion == "" {
			return fmt.Errorf("yaml must include apiVersion and kind")
		}
		if isNativeKind(kind) {
			return fmt.Errorf("kind %q is already supported natively", kind)
		}
	}
	if !found {
		return fmt.Errorf("yaml is required")
	}
	return nil
}

func isNativeKind(kind string) bool {
	switch kind {
	case "Namespace", "ConfigMap", "Secret", "PersistentVolumeClaim", "Deployment", "StatefulSet", "Service", "Job", "CronJob", "Ingress":
		return true
	default:
		return false
	}
}

func (model Model) validateSourceFile(source string) error {
	if source == "" {
		return fmt.Errorf("source is required")
	}
	source = model.sourcePath(source)
	if _, err := os.ReadFile(source); err != nil {
		return fmt.Errorf("read source %q: %w", source, err)
	}
	return nil
}

func (model Model) validateSecretSource(source string) error {
	if source == "" {
		return fmt.Errorf("source is required")
	}
	path := model.sourcePath(source)
	if _, err := godotenv.Read(path); err != nil {
		return fmt.Errorf("read source %q: %w", path, err)
	}
	return nil
}

func (model Model) sourcePath(source string) string {
	if filepath.IsAbs(source) {
		return source
	}
	return filepath.Join(filepath.Dir(model.SourcePath), source)
}
