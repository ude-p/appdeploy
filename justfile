set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

kubectl := "kubectl"

default:
  @just --list

help:
  @just --list

manifests:
  go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.21.0 rbac:roleName=manager-role crd webhook paths=./... output:crd:artifacts:config=config/crd/bases

generate:
  go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.21.0 object paths=./...

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

build-installer:
  mkdir -p dist
  "{{kubectl}}" kustomize config/default > dist/install.yaml

install:
  "{{kubectl}}" apply -k config/crd

uninstall:
  "{{kubectl}}" delete --ignore-not-found=false -k config/crd

deploy:
  "{{kubectl}}" apply -k config/default

undeploy:
  "{{kubectl}}" delete --ignore-not-found=false -k config/default
