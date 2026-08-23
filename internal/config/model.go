package config

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
)

type Model struct {
	Namespaces []Namespace `hcl:"namespace,block"`
	Configs    []Config    `hcl:"config,block"`
	Secrets    []SecretHCL `hcl:"secret,block"`
	Volumes    []Volume    `hcl:"volume,block"`
	Apps       []App       `hcl:"app,block"`
	Statefuls  []Stateful  `hcl:"stateful,block"`
	Jobs       []Job       `hcl:"job,block"`
	Crons      []Cron      `hcl:"cron,block"`
	Routes     []Route     `hcl:"route,block"`
	Manifests  []Manifest  `hcl:"manifest,block"`

	SourcePath string
}

func ParseHCL(path string) (Model, error) {
	parser := hclparse.NewParser()
	file, diagnostics := parser.ParseHCLFile(path)
	if diagnostics.HasErrors() {
		return Model{}, fmt.Errorf("parse config %q: %s", path, diagnostics.Error())
	}

	var model Model
	diagnostics = gohcl.DecodeBody(file.Body, nil, &model)
	if diagnostics.HasErrors() {
		return Model{}, fmt.Errorf("decode config %q: %s", path, diagnostics.Error())
	}
	model.SourcePath = path

	return model, nil
}

type Namespace struct {
	Name string `hcl:"name,label"`
}

func (model Model) HasNamespace(name string) bool {
	for _, namespace := range model.Namespaces {
		if namespace.Name == name {
			return true
		}
	}
	return false
}

type Config struct {
	Name   string       `hcl:"name,label"`
	Env    *EnvSource   `hcl:"env,block"`
	Files  []ConfigFile `hcl:"file,block"`
	Scope  *ConfigScope `hcl:"scope,block"`
	Values hcl.Body     `hcl:",remain"`
}

type ConfigFile struct {
	Name   string `hcl:"name,label"`
	Source string `hcl:"source,attr"`
}

type ConfigScope struct {
	Namespace string       `hcl:"namespace,label"`
	Env       *EnvSource   `hcl:"env,block"`
	Files     []ConfigFile `hcl:"file,block"`
	Values    hcl.Body     `hcl:",remain"`
}

type SecretHCL struct {
	Name  string       `hcl:"name,label"`
	Env   *EnvSource   `hcl:"env,block"`
	Files []SecretFile `hcl:"file,block"`
	Scope *SecretScope `hcl:"scope,block"`
}

type SecretScope struct {
	Namespace string       `hcl:"namespace,label"`
	Env       *EnvSource   `hcl:"env,block"`
	Files     []SecretFile `hcl:"file,block"`
}

type EnvSource struct {
	Source string `hcl:"source,attr"`
	Prefix string `hcl:"prefix,optional"`
}

type SecretFile struct {
	Name   string `hcl:"name,label"`
	Source string `hcl:"source,attr"`
}

type Volume struct {
	Name         string `hcl:"name,label"`
	Size         string `hcl:"size,attr"`
	StorageClass string `hcl:"storage_class,optional"`
	Access       string `hcl:"access,optional"`
}

type App struct {
	Name      string     `hcl:"name,label"`
	Image     string     `hcl:"image,attr"`
	Replicas  *int32     `hcl:"replicas,optional"`
	Command   []string   `hcl:"command,optional"`
	Args      []string   `hcl:"args,optional"`
	Env       *Env       `hcl:"env,block"`
	Ports     []Port     `hcl:"port,block"`
	Resources *Resources `hcl:"resources,block"`
	Mounts    []Mount    `hcl:"mount,block"`
	Health    *Health    `hcl:"health,block"`
}

type Stateful struct {
	Name    string           `hcl:"name,label"`
	Image   string           `hcl:"image,attr"`
	Env     *Env             `hcl:"env,block"`
	Ports   []Port           `hcl:"port,block"`
	Volumes []StatefulVolume `hcl:"volume,block"`
}

type StatefulVolume struct {
	Name         string `hcl:"name,label"`
	Size         string `hcl:"size,attr"`
	StorageClass string `hcl:"storage_class,optional"`
	Access       string `hcl:"access,optional"`
	Path         string `hcl:"path,attr"`
}

type Job struct {
	Name    string   `hcl:"name,label"`
	Image   string   `hcl:"image,attr"`
	Command []string `hcl:"command,optional"`
	Args    []string `hcl:"args,optional"`
	Retries *int32   `hcl:"retries,optional"`
	Env     *Env     `hcl:"env,block"`
}

type Cron struct {
	Name     string   `hcl:"name,label"`
	Image    string   `hcl:"image,attr"`
	Schedule string   `hcl:"schedule,attr"`
	Command  []string `hcl:"command,optional"`
	Args     []string `hcl:"args,optional"`
	Retries  *int32   `hcl:"retries,optional"`
	Env      *Env     `hcl:"env,block"`
}

type Env struct {
	Config hcl.Expression `hcl:"config,optional"`
	Secret hcl.Expression `hcl:"secret,optional"`
}

type Port struct {
	Service   int32 `hcl:"service,attr"`
	Container int32 `hcl:"container,optional"`
}

type Resources struct {
	CPU    string `hcl:"cpu,attr"`
	Memory string `hcl:"memory,attr"`
}

type Mount struct {
	Volume hcl.Expression `hcl:"volume,attr"`
	Path   string         `hcl:"path,attr"`
}

type Health struct {
	Path string `hcl:"path,attr"`
	Port int32  `hcl:"port,attr"`
}

type Route struct {
	Name        string            `hcl:"name,label"`
	To          hcl.Expression    `hcl:"to,attr"`
	Host        string            `hcl:"host,attr"`
	ClassName   string            `hcl:"class_name,optional"`
	Paths       []RoutePath       `hcl:"path,block"`
	TLSSecret   string            `hcl:"tls_secret,optional"`
	Annotations map[string]string `hcl:"annotations,optional"`
}

type RoutePath struct {
	Path string `hcl:"path,label"`
	Port *int32 `hcl:"port,optional"`
}

type Manifest struct {
	Name string `hcl:"name,label"`
	YAML string `hcl:"yaml,attr"`
}
