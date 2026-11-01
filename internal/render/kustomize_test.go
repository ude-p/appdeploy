package render

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ude-p/appdeploy/internal/config"
	corev1 "k8s.io/api/core/v1"
)

func TestKustomizeWritesNamespaceDirs(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "deploy.hcl", `
namespaces = ["prod"]

app "api" {
  image = "nginx"
  port  = 80

  env {
    config {
      LOG_LEVEL = "info"
    }
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

	out := filepath.Join(dir, "out")
	if err := Kustomize(model, out, []string{"prod"}); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{
		"namespace-prod.yaml",
		"configmap-api-config.yaml",
		"deployment-api.yaml",
		"service-api.yaml",
		"kustomization.yaml",
	} {
		if _, err := os.Stat(filepath.Join(out, "prod", name)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}

	content, err := os.ReadFile(filepath.Join(out, "prod", "kustomization.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"namespace-prod.yaml",
		"configmap-api-config.yaml",
		"deployment-api.yaml",
		"service-api.yaml",
	} {
		if !strings.Contains(string(content), name) {
			t.Fatalf("kustomization.yaml missing %s:\n%s", name, content)
		}
	}
}

func TestKustomizeEncryptsSecrets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake sops uses a shell script")
	}
	bin := t.TempDir()
	script := filepath.Join(bin, "sops")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nfor last; do :; done\nprintf '# encrypted\\n' >> \"$last\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	dir := t.TempDir()
	writeTestFile(t, dir, ".env", "SECRET_PASSWORD=pass\n")
	writeTestFile(t, dir, "deploy.hcl", `
namespaces = ["prod"]

app "api" {
  image = "nginx"

  env {
    secret {
      source = ".env"
      prefix = "SECRET_"
    }
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

	out := filepath.Join(dir, "out")
	if err := Kustomize(model, out, []string{"prod"}); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(out, "prod", "secret-api-secret.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "PASSWORD: cGFzcw==") {
		t.Fatalf("secret is not base64 data:\n%s", content)
	}
	if !strings.Contains(string(content), "# encrypted") {
		t.Fatalf("secret was not passed through sops:\n%s", content)
	}
}

func TestSecretWithData(t *testing.T) {
	secret := &corev1.Secret{StringData: map[string]string{"PASSWORD": "pass"}}
	got := secretWithData(secret)
	if string(got.Data["PASSWORD"]) != "pass" {
		t.Fatalf("data = %v", got.Data)
	}
	if got.StringData != nil {
		t.Fatalf("stringData = %v", got.StringData)
	}
	if secret.StringData == nil {
		t.Fatal("original secret was modified")
	}
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
