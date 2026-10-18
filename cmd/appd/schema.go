package main

import (
	"fmt"
	"os"
	"strings"
)

type schemaTopic struct {
	name    string
	summary string
	body    string
}

// Topics follow the Model order, with scope last because it applies across blocks.
// Bodies are raw strings so their HCL keeps its own indentation.
var (
	namespaceTopic = schemaTopic{
		name:    "namespace",
		summary: "Declare a namespace to render into",
		body: `
			namespace "prod" {}

			Declares a namespace AppDeploy can render into. At least one namespace is
			required. Every other block renders into one or more declared namespaces.

			Fields:
			  name (label)  required  namespace name
`,
	}

	configTopic = schemaTopic{
		name:    "config",
		summary: "Build a ConfigMap from values, files, and .env",
		body: `
			config "app" {
			  env {
			    source = ".env"
			    prefix = "CONFIG_"
			  }

			  LOG_LEVEL = "info"

			  data = {
			    "file.txt" = "content"
			  }

			  file "init.sql" {
			    source = "./init.sql"
			  }

			  scope "staging" {
			    LOG_LEVEL = "debug"
			  }
			}

			Builds a ConfigMap.

			Sources:
			  env     read a .env file; prefix is stripped from keys
			  inline  string attributes such as LOG_LEVEL = "info"
			  data    map of file name to content
			  file    read a real file; the label is the key
			  scope   per-namespace overrides, applied on top of the base

			A scoped file replaces the base file with the same name. The base
			definition stays in one place.
`,
	}

	secretTopic = schemaTopic{
		name:    "secret",
		summary: "Build a Secret from .env values and files",
		body: `
			secret "app" {
			  env {
			    source = ".env"
			    prefix = "SECRET_"
			  }

			  file "tls.crt" {
			    source = "./tls.crt"
			  }

			  scope "staging" {
			    env {
			      source = ".env.staging"
			      prefix = "SECRET_"
			    }
			  }
			}

			Builds a Secret. It uses the same sources as config, without inline
			values or data.

			Secrets are sent to Kubernetes as plain text (stringData). Keep
			appd run -o output out of version control.
`,
	}

	volumeTopic = schemaTopic{
		name:    "volume",
		summary: "Create a PersistentVolumeClaim",
		body: `
			volume "uploads" {
			  size          = "10Gi"
			  storage_class = "standard"
			  access        = "ReadWriteOnce"
			}

			Creates a PersistentVolumeClaim. Mount it with a mount block.

			Fields:
			  size           required  storage request
			  storage_class  optional  defaults to the cluster default
			  access         required  ReadWriteOnce, ReadWriteMany, or ReadOnlyMany
			                           There is no default; an empty value produces
			                           an invalid PVC.
`,
	}

	appTopic = schemaTopic{
		name:    "app",
		summary: "Create a Deployment and Service",
		body: `
			app "api" {
			  image    = "ghcr.io/example/api:1.4.0"
			  replicas = 2

			  node_selector = { "workload" = "general" }

			  command = []
			  args    = []

			  env {
			    config = config.app
			    secret = secret.app
			    field "POD_NAME" { path = "metadata.name" }
			  }

			  port {
			    service   = 80
			    container = 3000
			  }

			  resources {
			    cpu    = "500m"
			    memory = "512Mi"
			  }

			  mount {
			    config = config.app
			    path   = "/etc/app"
			  }

			  health {
			    path = "/health"
			    port = 3000
			  }
			}

			Creates a Deployment and, when at least one port is declared, a Service.

			Fields:
			  image          required  pulled with Always
			  replicas       optional  default 1
			  node_selector  optional  map of node labels
			  command, args  optional  override the image entrypoint
			  env            optional  ConfigMap and Secret references, plus field refs
			  port           optional  container defaults to the service port
			  resources      optional  requests and limits are set to the same values
			  mount          optional  exactly one of volume, config, or secret, plus path
			  health         optional  one HTTP GET used for liveness and readiness
			  scope          optional  render only into this namespace
`,
	}

	statefulTopic = schemaTopic{
		name:    "stateful",
		summary: "Create a StatefulSet and headless Service",
		body: `
			stateful "postgres" {
			  image = "postgres:16"

			  node_selector = { "workload" = "storage" }

			  env {
			    config = config.db
			  }

			  port {
			    service = 5432
			  }

			  volume "data" {
			    size          = "20Gi"
			    storage_class = "standard"
			    access        = "ReadWriteOnce"
			    path          = "/var/lib/postgresql/data"
			  }

			  mount {
			    config = config.init
			    path   = "/docker-entrypoint-initdb.d"
			  }
			}

			Creates a StatefulSet and a headless Service. Runs one replica.

			Fields:
			  image          required  pulled with Always
			  node_selector  optional  map of node labels
			  env            optional  ConfigMap and Secret references, plus field refs
			  port           optional  service port
			  volume         optional  becomes a PVC template; path is the mount path
			  mount          optional  exactly one of volume, config, or secret, plus path
			  scope          optional  render only into this namespace
`,
	}

	jobTopic = schemaTopic{
		name:    "job",
		summary: "Create a one-shot Job",
		body: `
			job "migrate" {
			  image   = "ghcr.io/example/api:1.4.0"
			  command = ["/app/migrate"]
			  args    = []
			  retries = 3

			  node_selector = { "workload" = "storage" }

			  env {
			    config = config.app
			  }

			  mount {
			    volume = volume.data
			    path   = "/data"
			  }
			}

			Creates a Job with restartPolicy Never. retries sets backoffLimit.

			Re-applying a completed Job does not run it again; delete the Job to
			run it again.
`,
	}

	cronTopic = schemaTopic{
		name:    "cron",
		summary: "Create a CronJob",
		body: `
			cron "cleanup" {
			  image    = "ghcr.io/example/api:1.4.0"
			  schedule = "0 2 * * *"
			  command  = ["/app/cleanup"]
			  args     = []
			  retries  = 3

			  node_selector = { "workload" = "storage" }

			  env {
			    config = config.app
			  }
			}

			Creates a CronJob with restartPolicy Never. retries sets backoffLimit.
			schedule uses the standard five-field cron format.
`,
	}

	routeTopic = schemaTopic{
		name:    "route",
		summary: "Create an Ingress and optional Traefik middleware",
		body: `
			route "api" {
			  to         = app.api
			  host       = "api.example.com"
			  class_name = "traefik"
			  tls_secret = "api-tls"

			  path "/" {}

			  path "/metrics" {
			    port = 9090
			  }

			  annotations = {
			    "example.com/owner" = "platform"
			  }

			  middleware {
			    headers {
			      allow_origins     = ["https://app.example.com"]
			      allow_methods     = ["GET", "POST"]
			      allow_headers     = ["Content-Type"]
			      allow_credentials = true
			    }
			  }
			}

			Creates an Ingress for an app. to must reference an app with a port.
			A path without port uses the app's first service port. Paths are
			Prefix type.

			middleware creates a Traefik Middleware named <route>-middleware and
			adds the router annotation to the Ingress. allow_origins and
			allow_methods are required. Traefik CRDs must be installed.
`,
	}

	manifestTopic = schemaTopic{
		name:    "manifest",
		summary: "Add raw Kubernetes YAML",
		body: `
			manifest "widget" {
			  yaml = <<YAML
			apiVersion: example.com/v1
			kind: Widget
			metadata:
			  name: widget
			  namespace: prod
			spec: {}
			YAML
			}

			Adds raw Kubernetes YAML for resources AppDeploy does not model.
			Each document needs apiVersion and kind. Native kinds are rejected.

			Manifests render once, not per namespace, so include metadata.namespace
			yourself.
`,
	}

	scopeTopic = schemaTopic{
		name:    "scope",
		summary: "How a block chooses its namespace",
		body: `
			scope = "staging"

			Every block except namespace, volume, and manifest accepts scope.

			Without scope, the block renders into every declared namespace.
			With scope, the block renders only into that namespace.

			config and secret scope blocks override base values; a scoped file
			replaces the base file with the same name. appd run -n <namespace>
			renders one namespace at a time.
`,
	}
)

var schemaTopics = []schemaTopic{
	namespaceTopic,
	configTopic,
	secretTopic,
	volumeTopic,
	appTopic,
	statefulTopic,
	jobTopic,
	cronTopic,
	routeTopic,
	manifestTopic,
	scopeTopic,
}

func explain(args []string) {
	switch len(args) {
	case 0:
		printSchemaIndex()
	case 1:
		topic, ok := findSchemaTopic(args[0])
		if !ok {
			fail(fmt.Errorf("unknown topic %q; available: %s", args[0], strings.Join(schemaTopicNames(), ", ")))
		}
		mustFprintln(os.Stdout, strings.TrimSpace(dedent(topic.body)))
	default:
		fail(fmt.Errorf("explain accepts at most one topic"))
	}
}

// dedent removes the common indentation of the raw schema body.
func dedent(s string) string {
	lines := strings.Split(s, "\n")
	indent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		n := len(line) - len(strings.TrimLeft(line, "\t"))
		if indent == -1 || n < indent {
			indent = n
		}
	}
	if indent <= 0 {
		return s
	}
	for i := range lines {
		if len(lines[i]) >= indent {
			lines[i] = lines[i][indent:]
		}
	}
	return strings.Join(lines, "\n")
}

func printSchemaIndex() {
	var index strings.Builder
	index.WriteString("AppDeploy schema\n\nTopics:\n")
	for _, topic := range schemaTopics {
		fmt.Fprintf(&index, "  %-11s %s\n", topic.name, topic.summary)
	}
	index.WriteString("\nRun \"appd explain <topic>\" for details.\n")
	mustFprintln(os.Stdout, strings.TrimSpace(index.String()))
}

func findSchemaTopic(name string) (schemaTopic, bool) {
	for _, topic := range schemaTopics {
		if topic.name == name {
			return topic, true
		}
	}
	return schemaTopic{}, false
}

func schemaTopicNames() []string {
	names := make([]string, 0, len(schemaTopics))
	for _, topic := range schemaTopics {
		names = append(names, topic.name)
	}
	return names
}
