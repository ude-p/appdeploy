package config

import (
	"fmt"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

type Model struct {
	Namespaces []string  `hcl:"namespaces,optional"`
	Defaults   *Defaults `hcl:"defaults,block"`
	Configs    []Config    `hcl:"config,block"`
	Secrets    []Secret    `hcl:"secret,block"`
	Volumes    []Volume    `hcl:"volume,block"`
	Apps       []App       `hcl:"app,block"`
	Statefuls  []Stateful  `hcl:"stateful,block"`
	Jobs       []Job       `hcl:"job,block"`
	Manifests  []Manifest  `hcl:"manifest,block"`

	SourcePath string
}

func (model Model) HasNamespace(name string) bool {
	return slices.Contains(model.Namespaces, name)
}

type Defaults struct {
	Replicas           *int32            `hcl:"replicas,optional"`
	NodeSelector       map[string]string `hcl:"node_selector,optional"`
	IngressClass       string            `hcl:"ingress_class,optional"`
	IngressAnnotations map[string]string `hcl:"ingress_annotations,optional"`
	Resources          *Resources        `hcl:"resources,block"`
}

type Resources struct {
	CPU    string `hcl:"cpu,attr"`
	Memory string `hcl:"memory,attr"`
}

type Config struct {
	Name   string   `hcl:"name,label"`
	Source string   `hcl:"source,optional"`
	Prefix string   `hcl:"prefix,optional"`
	Files  []File   `hcl:"file,block"`
	Scopes []Scope  `hcl:"scope,block"`
	Values hcl.Body `hcl:",remain"`
}

type Secret struct {
	Name   string  `hcl:"name,label"`
	Source string  `hcl:"source,optional"`
	Prefix string  `hcl:"prefix,optional"`
	Files  []File  `hcl:"file,block"`
	Scopes []Scope `hcl:"scope,block"`
}

type Scope struct {
	Namespace string   `hcl:"namespace,label"`
	Source    string   `hcl:"source,optional"`
	Prefix    string   `hcl:"prefix,optional"`
	Files     []File   `hcl:"file,block"`
	Values    hcl.Body `hcl:",remain"`
}

type File struct {
	Name   string `hcl:"name,label"`
	Source string `hcl:"source,attr"`
}

type Volume struct {
	Name         string `hcl:"name,label"`
	Size         string `hcl:"size,attr"`
	StorageClass string `hcl:"storage_class,optional"`
	Access       string `hcl:"access,optional"`
}

type Env struct {
	Config     *EnvValues     `hcl:"config,block"`
	Secret     *EnvValues     `hcl:"secret,block"`
	FromConfig hcl.Expression `hcl:"from_config,optional"`
	FromSecret hcl.Expression `hcl:"from_secret,optional"`
	Values     hcl.Body       `hcl:",remain"`
}

type EnvValues struct {
	Source string   `hcl:"source,optional"`
	Prefix string   `hcl:"prefix,optional"`
	Files  []File   `hcl:"file,block"`
	Scopes []Scope  `hcl:"scope,block"`
	Values hcl.Body `hcl:",remain"`
}

type Mount struct {
	Volume hcl.Expression `hcl:"volume,optional"`
	Config hcl.Expression `hcl:"config,optional"`
	Secret hcl.Expression `hcl:"secret,optional"`
	Path   string         `hcl:"path,attr"`
}

type App struct {
	Name         string            `hcl:"name,label"`
	Scope        string            `hcl:"scope,optional"`
	Image        string            `hcl:"image,attr"`
	Replicas     *int32            `hcl:"replicas,optional"`
	NodeSelector map[string]string `hcl:"node_selector,optional"`
	Command      []string          `hcl:"command,optional"`
	Args         []string          `hcl:"args,optional"`
	Env          *Env              `hcl:"env,block"`
	Port         int32             `hcl:"port,optional"`
	Ports        []int32           `hcl:"ports,optional"`
	Resources    *Resources        `hcl:"resources,block"`
	Mounts       []Mount           `hcl:"mount,block"`
	Health       *Health           `hcl:"health,block"`
	Routes       []Route           `hcl:"route,block"`
}

func (app App) ServicePorts() []int32 {
	return servicePorts(app.Port, app.Ports)
}

type Health struct {
	Path string `hcl:"path,attr"`
	Port int32  `hcl:"port,optional"`
}

type Stateful struct {
	Name         string            `hcl:"name,label"`
	Scope        string            `hcl:"scope,optional"`
	Image        string            `hcl:"image,attr"`
	NodeSelector map[string]string `hcl:"node_selector,optional"`
	Env          *Env              `hcl:"env,block"`
	Port         int32             `hcl:"port,optional"`
	Ports        []int32           `hcl:"ports,optional"`
	Volumes      []StatefulVolume  `hcl:"volume,block"`
	Mounts       []Mount           `hcl:"mount,block"`
	Health       *Health           `hcl:"health,block"`
}

func (stateful Stateful) ServicePorts() []int32 {
	return servicePorts(stateful.Port, stateful.Ports)
}

type StatefulVolume struct {
	Name         string `hcl:"name,label"`
	Size         string `hcl:"size,attr"`
	StorageClass string `hcl:"storage_class,optional"`
	Access       string `hcl:"access,optional"`
	Path         string `hcl:"path,attr"`
}

type Job struct {
	Name         string            `hcl:"name,label"`
	Scope        string            `hcl:"scope,optional"`
	Image        string            `hcl:"image,attr"`
	NodeSelector map[string]string `hcl:"node_selector,optional"`
	Schedule     string            `hcl:"schedule,optional"`
	Command      []string          `hcl:"command,optional"`
	Args         []string          `hcl:"args,optional"`
	Retries      *int32            `hcl:"retries,optional"`
	Env          *Env              `hcl:"env,block"`
	Mounts       []Mount           `hcl:"mount,block"`
}

type Route struct {
	Name        string            `hcl:"name,label"`
	Scope       string            `hcl:"scope,optional"`
	Host        string            `hcl:"host,attr"`
	Path        string            `hcl:"path,optional"`
	Paths       []string          `hcl:"paths,optional"`
	Port        *int32            `hcl:"port,optional"`
	TLS         string            `hcl:"tls,optional"`
	Annotations map[string]string `hcl:"annotations,optional"`
}

type Manifest struct {
	Name string `hcl:"name,label"`
	YAML string `hcl:"yaml,attr"`
}

var DownwardEnv = map[string]string{
	"POD_NAME":      "metadata.name",
	"POD_NAMESPACE": "metadata.namespace",
	"POD_IP":        "status.podIP",
	"POD_UID":       "metadata.uid",
	"NODE_NAME":     "spec.nodeName",
	"HOST_IP":       "status.hostIP",
}

func HasExpression(expression hcl.Expression) bool {
	if expression == nil {
		return false
	}
	result, diagnostics := expression.Value(nil)
	return diagnostics.HasErrors() || !result.IsNull()
}

func StringValues(body hcl.Body, skip ...string) (map[string]string, error) {
	data := make(map[string]string)
	if body == nil {
		return data, nil
	}
	syntaxBody, ok := body.(*hclsyntax.Body)
	if !ok {
		return nil, fmt.Errorf("read values")
	}
	excluded := make(map[string]struct{}, len(skip))
	for _, name := range skip {
		excluded[name] = struct{}{}
	}
	for name, attribute := range syntaxBody.Attributes {
		if _, ok := excluded[name]; ok {
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

func servicePorts(port int32, ports []int32) []int32 {
	if port != 0 {
		return []int32{port}
	}
	return ports
}
