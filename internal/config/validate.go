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
		if err := addName(namespaces, "namespace", namespace); err != nil {
			return err
		}
	}
	if len(namespaces) == 0 {
		return fmt.Errorf("at least one namespace is required")
	}
	if model.Defaults != nil && model.Defaults.Replicas != nil && *model.Defaults.Replicas < 0 {
		return fmt.Errorf("defaults: replicas must be positive")
	}

	configs := make(map[string]struct{}, len(model.Configs))
	for _, config := range model.Configs {
		if err := addName(configs, "config", config.Name); err != nil {
			return err
		}
		spec := valuesSpec{
			Kind:     "config",
			Source:   config.Source,
			Prefix:   config.Prefix,
			Files:    config.Files,
			Scopes:   config.Scopes,
			Body:     config.Values,
			Skip:     []string{"source", "prefix"},
			Literals: true,
		}
		if err := model.validateValues(spec, namespaces); err != nil {
			return fmt.Errorf("config %q: %w", config.Name, err)
		}
	}

	secrets := make(map[string]struct{}, len(model.Secrets))
	for _, secret := range model.Secrets {
		if err := addName(secrets, "secret", secret.Name); err != nil {
			return err
		}
		spec := valuesSpec{
			Kind:   "secret",
			Source: secret.Source,
			Prefix: secret.Prefix,
			Files:  secret.Files,
			Scopes: secret.Scopes,
			Skip:   []string{"source", "prefix"},
		}
		if err := model.validateValues(spec, namespaces); err != nil {
			return fmt.Errorf("secret %q: %w", secret.Name, err)
		}
	}

	volumes := make(map[string]struct{}, len(model.Volumes))
	for _, volume := range model.Volumes {
		if err := addName(volumes, "volume", volume.Name); err != nil {
			return err
		}
	}

	generated := &generatedNames{
		configs: make(map[string]string),
		secrets: make(map[string]string),
	}

	apps := make(map[string]struct{}, len(model.Apps))
	routes := make(map[string]struct{})
	for _, app := range model.Apps {
		if err := addName(apps, "app", app.Name); err != nil {
			return err
		}
		if err := validateHCLScope(app.Scope, namespaces); err != nil {
			return fmt.Errorf("app %q: %w", app.Name, err)
		}
		if err := model.validateEnv("app", app.Name, app.Env, namespaces, configs, secrets, generated); err != nil {
			return fmt.Errorf("app %q: %w", app.Name, err)
		}
		if err := validatePorts(app.Port, app.Ports); err != nil {
			return fmt.Errorf("app %q: %w", app.Name, err)
		}
		if err := validateHealth(app.Health, app.ServicePorts()); err != nil {
			return fmt.Errorf("app %q: %w", app.Name, err)
		}
		for _, mount := range app.Mounts {
			if err := validateMount(mount, volumes, configs, secrets); err != nil {
				return fmt.Errorf("app %q mount: %w", app.Name, err)
			}
		}
		for _, route := range app.Routes {
			if err := validateHCLScope(route.Scope, namespaces); err != nil {
				return fmt.Errorf("app %q route %q: %w", app.Name, route.Name, err)
			}
			if err := validateRoute(app, route, routes); err != nil {
				return fmt.Errorf("app %q route %q: %w", app.Name, route.Name, err)
			}
		}
	}

	statefuls := make(map[string]struct{}, len(model.Statefuls))
	for _, stateful := range model.Statefuls {
		if err := addName(statefuls, "stateful", stateful.Name); err != nil {
			return err
		}
		if err := validateHCLScope(stateful.Scope, namespaces); err != nil {
			return fmt.Errorf("stateful %q: %w", stateful.Name, err)
		}
		if err := model.validateEnv("stateful", stateful.Name, stateful.Env, namespaces, configs, secrets, generated); err != nil {
			return fmt.Errorf("stateful %q: %w", stateful.Name, err)
		}
		if err := validatePorts(stateful.Port, stateful.Ports); err != nil {
			return fmt.Errorf("stateful %q: %w", stateful.Name, err)
		}
		if err := validateHealth(stateful.Health, stateful.ServicePorts()); err != nil {
			return fmt.Errorf("stateful %q: %w", stateful.Name, err)
		}
		for _, mount := range stateful.Mounts {
			if err := validateMount(mount, volumes, configs, secrets); err != nil {
				return fmt.Errorf("stateful %q mount: %w", stateful.Name, err)
			}
		}
	}

	if err := model.validateJobs(namespaces, volumes, configs, secrets, generated); err != nil {
		return err
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

type valuesSpec struct {
	Kind     string
	Source   string
	Prefix   string
	Files    []File
	Scopes   []Scope
	Body     hcl.Body
	Skip     []string
	Literals bool
}

func (model Model) validateValues(spec valuesSpec, namespaces map[string]struct{}) error {
	if spec.Source != "" {
		if err := model.validateSourceFile(spec.Source); err != nil {
			return err
		}
	}

	fileNames := make(map[string]struct{}, len(spec.Files))
	for _, file := range spec.Files {
		if err := addName(fileNames, spec.Kind+" file", file.Name); err != nil {
			return err
		}
		if err := model.validateSourceFile(file.Source); err != nil {
			return fmt.Errorf("file %q: %w", file.Name, err)
		}
	}

	values, err := StringValues(spec.Body, spec.Skip...)
	if err != nil {
		return err
	}
	if !spec.Literals && len(values) > 0 {
		return fmt.Errorf("%s values are not allowed", spec.Kind)
	}
	if spec.Literals && spec.Source != "" && len(values) > 0 {
		envValues, err := godotenv.Read(model.sourcePath(spec.Source))
		if err != nil {
			return fmt.Errorf("read env source: %w", err)
		}
		for name := range values {
			if _, exists := envValues[spec.Prefix+name]; exists {
				return fmt.Errorf("value %q is already provided by env source", name)
			}
		}
	}
	if spec.Source == "" && len(values) == 0 && len(spec.Files) == 0 {
		return fmt.Errorf("at least one of source, values, or file is required")
	}

	for _, scope := range spec.Scopes {
		if err := validateHCLScope(scope.Namespace, namespaces); err != nil {
			return err
		}
		if scope.Source != "" {
			if err := model.validateSourceFile(scope.Source); err != nil {
				return fmt.Errorf("scope %q: %w", scope.Namespace, err)
			}
		}
		scopeFileNames := make(map[string]struct{}, len(scope.Files))
		for _, file := range scope.Files {
			if err := addName(scopeFileNames, spec.Kind+" file", file.Name); err != nil {
				return fmt.Errorf("scope %q: %w", scope.Namespace, err)
			}
			if _, exists := fileNames[file.Name]; !exists {
				return fmt.Errorf("scope %q: file %q is not declared in the base %s", scope.Namespace, file.Name, spec.Kind)
			}
			if err := model.validateSourceFile(file.Source); err != nil {
				return fmt.Errorf("scope %q file %q: %w", scope.Namespace, file.Name, err)
			}
		}
		scopeValues, err := StringValues(scope.Values, "source", "prefix")
		if err != nil {
			return fmt.Errorf("scope %q: %w", scope.Namespace, err)
		}
		if !spec.Literals && len(scopeValues) > 0 {
			return fmt.Errorf("scope %q: %s values are not allowed", scope.Namespace, spec.Kind)
		}
	}
	return nil
}

func (model Model) validateJobs(namespaces, volumes, configs, secrets map[string]struct{}, generated *generatedNames) error {
	names := make(map[string]struct{}, len(model.Jobs))
	for _, job := range model.Jobs {
		if err := addName(names, "job", job.Name); err != nil {
			return err
		}
		if err := validateHCLScope(job.Scope, namespaces); err != nil {
			return fmt.Errorf("job %q: %w", job.Name, err)
		}
		if err := model.validateEnv("job", job.Name, job.Env, namespaces, configs, secrets, generated); err != nil {
			return fmt.Errorf("job %q: %w", job.Name, err)
		}
		for _, mount := range job.Mounts {
			if err := validateMount(mount, volumes, configs, secrets); err != nil {
				return fmt.Errorf("job %q mount: %w", job.Name, err)
			}
		}
	}
	return nil
}

type generatedNames struct {
	configs map[string]string
	secrets map[string]string
}

func (generated *generatedNames) claim(kind, name, owner string, declared map[string]struct{}) error {
	names := generated.configs
	if kind == "secret" {
		names = generated.secrets
	}
	if _, exists := declared[name]; exists {
		return fmt.Errorf("generated %s %q collides with a declared %s block", kind, name, kind)
	}
	if previous, exists := names[name]; exists {
		return fmt.Errorf("generated %s %q is already used by %s", kind, name, previous)
	}
	names[name] = owner
	return nil
}

func (model Model) validateEnv(kind, name string, env *Env, namespaces, configs, secrets map[string]struct{}, generated *generatedNames) error {
	if env == nil {
		return nil
	}
	owner := kind + " " + name
	if env.Config != nil {
		generatedName := name + "-config"
		if err := generated.claim("config", generatedName, owner, configs); err != nil {
			return err
		}
		spec := valuesSpec{
			Kind:     "config",
			Source:   env.Config.Source,
			Prefix:   env.Config.Prefix,
			Files:    env.Config.Files,
			Scopes:   env.Config.Scopes,
			Body:     env.Config.Values,
			Skip:     []string{"source", "prefix"},
			Literals: true,
		}
		if err := model.validateValues(spec, namespaces); err != nil {
			return fmt.Errorf("config: %w", err)
		}
	}
	if env.Secret != nil {
		generatedName := name + "-secret"
		if err := generated.claim("secret", generatedName, owner, secrets); err != nil {
			return err
		}
		spec := valuesSpec{
			Kind:   "secret",
			Source: env.Secret.Source,
			Prefix: env.Secret.Prefix,
			Files:  env.Secret.Files,
			Scopes: env.Secret.Scopes,
			Body:   env.Secret.Values,
			Skip:   []string{"source", "prefix"},
		}
		if err := model.validateValues(spec, namespaces); err != nil {
			return fmt.Errorf("secret: %w", err)
		}
	}
	if err := validateReference(env.FromConfig, "config", configs); err != nil {
		return fmt.Errorf("from_config: %w", err)
	}
	if err := validateReference(env.FromSecret, "secret", secrets); err != nil {
		return fmt.Errorf("from_secret: %w", err)
	}
	values, err := StringValues(env.Values, "from_config", "from_secret")
	if err != nil {
		return err
	}
	for name := range values {
		if _, reserved := DownwardEnv[name]; reserved {
			return fmt.Errorf("environment variable %q is provided automatically", name)
		}
	}
	return nil
}

func validateRoute(app App, route Route, names map[string]struct{}) error {
	if err := addName(names, "route", route.Name); err != nil {
		return err
	}
	if strings.TrimSpace(route.Host) == "" {
		return fmt.Errorf("host is required")
	}
	if route.Path != "" && len(route.Paths) > 0 {
		return fmt.Errorf("use path or paths, not both")
	}
	paths := route.Paths
	if route.Path != "" {
		paths = []string{route.Path}
	}
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if path == "" || path[0] != '/' {
			return fmt.Errorf("path %q must start with /", path)
		}
		if _, exists := seen[path]; exists {
			return fmt.Errorf("path %q is declared more than once", path)
		}
		seen[path] = struct{}{}
	}
	if route.Port != nil && *route.Port <= 0 {
		return fmt.Errorf("port must be positive")
	}
	if route.Port == nil && len(app.ServicePorts()) == 0 {
		return fmt.Errorf("app has no port and route has no port")
	}
	return nil
}

func validateHealth(health *Health, ports []int32) error {
	if health == nil {
		return nil
	}
	if strings.TrimSpace(health.Path) == "" {
		return fmt.Errorf("health path is required")
	}
	if health.Port < 0 {
		return fmt.Errorf("health port must be positive")
	}
	if health.Port == 0 && len(ports) == 0 {
		return fmt.Errorf("health port is required when there is no port")
	}
	return nil
}

func validatePorts(port int32, ports []int32) error {
	if port != 0 && len(ports) > 0 {
		return fmt.Errorf("use port or ports, not both")
	}
	if port < 0 {
		return fmt.Errorf("port must be positive")
	}
	seen := make(map[int32]struct{}, len(ports)+1)
	if port != 0 {
		seen[port] = struct{}{}
	}
	for _, value := range ports {
		if value <= 0 {
			return fmt.Errorf("port %d must be positive", value)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("port %d is declared more than once", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateMount(mount Mount, volumes, configs, secrets map[string]struct{}) error {
	count := 0
	if HasExpression(mount.Volume) {
		count++
	}
	if HasExpression(mount.Config) {
		count++
	}
	if HasExpression(mount.Secret) {
		count++
	}
	if count != 1 {
		return fmt.Errorf("must specify exactly one of volume, config, or secret")
	}
	if HasExpression(mount.Volume) {
		return validateReference(mount.Volume, "volume", volumes)
	}
	if HasExpression(mount.Config) {
		return validateReference(mount.Config, "config", configs)
	}
	return validateReference(mount.Secret, "secret", secrets)
}

func validateHCLScope(namespace string, namespaces map[string]struct{}) error {
	if namespace == "" {
		return nil
	}
	if _, exists := namespaces[namespace]; !exists {
		return fmt.Errorf("scope references unknown namespace %q", namespace)
	}
	return nil
}

func validateReference(expression hcl.Expression, kind string, names map[string]struct{}) error {
	if !HasExpression(expression) {
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

func (model Model) sourcePath(source string) string {
	if filepath.IsAbs(source) {
		return source
	}
	return filepath.Join(filepath.Dir(model.SourcePath), source)
}
