set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

img := "controller:latest"
localbin := "bin"
kustomize := "{{localbin}}/kustomize"
controller_gen := "{{localbin}}/controller-gen"
golangci_lint := "{{localbin}}/golangci-lint"
kubectl := "kubectl"

default:
  @just --list

help:
  @just --list

manifests:
  "{{controller_gen}}" rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

generate:
  "{{controller_gen}}" object:paths="./..."

fmt:
  go fmt ./...

vet:
  go vet ./...

test:
  go test ./... -coverprofile cover.out

build:
  go build -o bin/manager cmd/main.go

run:
  go run ./cmd/main.go

lint:
  "{{golangci_lint}}" run

lint-fix:
  "{{golangci_lint}}" run --fix

build-installer:
  mkdir -p dist
  cd config/manager && "{{kustomize}}" edit set image controller={{img}}
  "{{kustomize}}" build config/default > dist/install.yaml

install:
  out="$({{kustomize}} build config/crd 2>/dev/null || true)"; \
  if [ -n "$out" ]; then echo "$out" | "{{kubectl}}" apply -f -; else echo "No CRDs to install; skipping."; fi

uninstall:
  out="$({{kustomize}} build config/crd 2>/dev/null || true)"; \
  if [ -n "$out" ]; then echo "$out" | "{{kubectl}}" delete --ignore-not-found=false -f -; else echo "No CRDs to delete; skipping."; fi

deploy:
  cd config/manager && "{{kustomize}}" edit set image controller={{img}}
  "{{kustomize}}" build config/default | "{{kubectl}}" apply -f -

undeploy:
  "{{kustomize}}" build config/default | "{{kubectl}}" delete --ignore-not-found=false -f -
