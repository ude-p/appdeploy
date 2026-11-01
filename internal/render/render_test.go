package render_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ude-p/appdeploy/internal/config"
	"github.com/ude-p/appdeploy/internal/render"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestBuildInlineEnv(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".env", "CONFIG_OTHER=x\nSHARED_TOKEN=abc\nSECRET_PASSWORD=pass\n")
	writeFile(t, dir, "features.json", `{"a":1}`)
	writeFile(t, dir, "deploy.hcl", `
namespaces = ["prod"]

config "shared" {
  source = ".env"
  prefix = "SHARED_"
}

app "api" {
  image = "nginx"

  env {
    APP_ENV = "production"

    config {
      source = ".env"
      prefix = "CONFIG_"

      LOG_LEVEL = "info"

      file "features.json" {
        source = "./features.json"
      }
    }

    secret {
      source = ".env"
      prefix = "SECRET_"
    }

    from_config = config.shared
  }

  port = 80
}
`)

	model, err := config.ParseHCL(filepath.Join(dir, "deploy.hcl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}
	objects, err := render.Build(model, "prod")
	if err != nil {
		t.Fatal(err)
	}

	configMaps := map[string]map[string]string{}
	secrets := map[string]map[string]string{}
	var deployment *appsv1.Deployment
	for _, object := range objects {
		switch item := object.(type) {
		case *corev1.ConfigMap:
			configMaps[item.Name] = item.Data
		case *corev1.Secret:
			secrets[item.Name] = item.StringData
		case *appsv1.Deployment:
			deployment = item
		}
	}

	wantConfig := map[string]string{
		"OTHER":         "x",
		"LOG_LEVEL":     "info",
		"features.json": `{"a":1}`,
	}
	if got := configMaps["api-config"]; !equal(got, wantConfig) {
		t.Fatalf("api-config = %v, want %v", got, wantConfig)
	}
	if got := configMaps["shared"]; !equal(got, map[string]string{"TOKEN": "abc"}) {
		t.Fatalf("shared = %v", got)
	}
	if got := secrets["api-secret"]; !equal(got, map[string]string{"PASSWORD": "pass"}) {
		t.Fatalf("api-secret = %v", got)
	}
	if deployment == nil {
		t.Fatal("deployment not built")
	}

	container := deployment.Spec.Template.Spec.Containers[0]
	env := map[string]corev1.EnvVar{}
	for _, item := range container.Env {
		env[item.Name] = item
	}
	if env["APP_ENV"].Value != "production" {
		t.Fatalf("APP_ENV = %v", env["APP_ENV"])
	}
	if ref := env["POD_NAME"].ValueFrom; ref == nil || ref.FieldRef == nil || ref.FieldRef.FieldPath != "metadata.name" {
		t.Fatalf("POD_NAME = %v", env["POD_NAME"])
	}
	if len(env) != 1+len(config.DownwardEnv) {
		t.Fatalf("env = %v", container.Env)
	}
	if probe := container.LivenessProbe; probe == nil || probe.TCPSocket == nil || probe.TCPSocket.Port.IntValue() != 80 {
		t.Fatalf("liveness probe = %v", probe)
	}
	wantRefs := []string{"api-config", "api-secret", "shared"}
	if len(container.EnvFrom) != len(wantRefs) {
		t.Fatalf("envFrom = %v", container.EnvFrom)
	}
	for i, want := range wantRefs {
		source := container.EnvFrom[i]
		var got string
		if source.ConfigMapRef != nil {
			got = source.ConfigMapRef.Name
		} else if source.SecretRef != nil {
			got = source.SecretRef.Name
		}
		if got != want {
			t.Fatalf("envFrom[%d] = %q, want %q", i, got, want)
		}
	}
}

func TestBuildStatefulProbes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "deploy.hcl", `
namespaces = ["prod"]

stateful "db" {
  image = "postgres"
  port  = 5432

  volume "data" {
    size   = "1Gi"
    access = "ReadWriteOnce"
    path   = "/data"
  }
}
`)

	model, err := config.ParseHCL(filepath.Join(dir, "deploy.hcl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}
	objects, err := render.Build(model, "prod")
	if err != nil {
		t.Fatal(err)
	}

	for _, object := range objects {
		stateful, ok := object.(*appsv1.StatefulSet)
		if !ok {
			continue
		}
		container := stateful.Spec.Template.Spec.Containers[0]
		for name, probe := range map[string]*corev1.Probe{
			"liveness":  container.LivenessProbe,
			"readiness": container.ReadinessProbe,
		} {
			if probe == nil || probe.TCPSocket == nil || probe.TCPSocket.Port.IntValue() != 5432 {
				t.Fatalf("%s probe = %v", name, probe)
			}
		}
		return
	}
	t.Fatal("statefulset not built")
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func equal(got, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for key, value := range want {
		if got[key] != value {
			return false
		}
	}
	return true
}
