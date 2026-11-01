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
	namespacesTopic = schemaTopic{
		name:    "namespaces",
		summary: "List the namespaces to render into",
		body: `
			namespaces = ["prod", "staging"]

			Lists the namespaces AppDeploy renders into. At least one is required.
			Every other block renders into all of them unless it sets scope.
`,
	}

	defaultsTopic = schemaTopic{
		name:    "defaults",
		summary: "Set defaults shared by workloads",
		body: `
			defaults {
			  replicas = 2

			  node_selector = { "workload" = "general" }

			  ingress_class = "traefik"

			  ingress_annotations = {
			    "cert-manager.io/cluster-issuer" = "letsencrypt"
			  }

			  resources {
			    cpu    = "250m"
			    memory = "256Mi"
			  }
			}

			Sets values used when a workload does not declare its own. app blocks
			override replicas, resources, and node_selector. ingress_class and
			ingress_annotations apply to every route; route annotations override
			them.
`,
	}

	configTopic = schemaTopic{
		name:    "config",
		summary: "Build a ConfigMap from values, files, and .env",
		body: `
			config "server-config" {
			  source = "./secrets/.env"
			  prefix = "CONFIG_"

			  LOG_LEVEL = "info"

			  file "init.sql" {
			    source = "./init.sql"
			  }

			  scope "staging" {
			    LOG_LEVEL = "debug"
			  }
			}

			Builds a ConfigMap.

			Sources:
			  source  read a .env file; prefix is stripped from keys
			  values  string attributes such as LOG_LEVEL = "info"
			  file    read a real file; the label is the key
			  scope   per-namespace overrides, applied on top of the base

			A scoped file replaces the base file with the same name.

			Declare config inside a workload env block to generate it as
			<workload>-config. Declare it here to share it across workloads.
`,
	}

	secretTopic = schemaTopic{
		name:    "secret",
		summary: "Build a Secret from .env values and files",
		body: `
			secret "server-secret" {
			  source = "./secrets/.env"
			  prefix = "SECRET_"

			  file "tls.crt" {
			    source = "./tls.crt"
			  }

			  scope "staging" {
			    source = "./secrets/.env.staging"
			    prefix = "SECRET_"
			  }
			}

			Builds a Secret. It uses the same sources as config, without
			inline values.

			Declare secret inside a workload env block to generate it as
			<workload>-secret. Declare it here to share it across workloads.

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
			    LOG_LEVEL = "info"

			    config {
			      source = "./secrets/.env"
			      prefix = "CONFIG_"
			    }
			  }

			  port = 80

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
			  }

			  route "api" {
			    host  = "api.example.com"
			    paths = ["/"]
			    tls   = "api-tls"
			  }
			}

			Creates a Deployment and, when at least one port is declared, a Service.
			Each route creates an Ingress for this app.

			Every container receives POD_NAME, POD_NAMESPACE, POD_IP, POD_UID,
			NODE_NAME, and HOST_IP. Do not declare them in env.

			Fields:
			  image          required  pulled with Always
			  replicas       optional  default 1 or defaults.replicas
			  node_selector  optional  map of node labels
			  command, args  optional  override the image entrypoint
			  env            optional  inline values, config, and secret
			  port           optional  one port; the container port matches
			  ports          optional  several ports, as a list
			  resources      optional  default defaults.resources; requests and
			                           limits are set to the same values
			  mount          optional  exactly one of volume, config, or secret, plus path
			  health         optional  HTTP probe; port defaults to the first port.
			                           Without this block the first port gets a TCP probe
			  route          optional  Ingress for this app; repeatable
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
			    from_config = config.db
			  }

			  port = 5432

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

			  health {
			    path = "/health"
			  }
			}

			Creates a StatefulSet and a headless Service. Runs one replica.

			Fields:
			  image          required  pulled with Always
			  node_selector  optional  map of node labels
			  env            optional  inline values, config, and secret
			  port           optional  one port; the container port matches
			  ports          optional  several ports, as a list
			  volume         optional  becomes a PVC template; path is the mount path
			  mount          optional  exactly one of volume, config, or secret, plus path
			  health         optional  HTTP probe; port defaults to the first port.
			                           Without this block the first port gets a TCP probe
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
			    from_config = config.app
			  }

			  mount {
			    volume = volume.data
			    path   = "/data"
			  }
			}

			Creates a Job with restartPolicy Never. retries sets backoffLimit.
			With schedule, creates a CronJob instead; schedule uses the standard
			five-field cron format.

			Re-applying a completed Job does not run it again; delete the Job to
			run it again.
`,
	}

	routeTopic = schemaTopic{
		name:    "route",
		summary: "Create an Ingress for an app",
		body: `
			app "api" {
			  image = "ghcr.io/example/api:1.4.0"
			  port  = 80

			  route "api" {
			    host  = "api.example.com"
			    paths = ["/", "/metrics"]
			    tls   = "api-tls"

			    annotations = {
			      "example.com/owner" = "platform"
			    }
			  }
			}

			Creates an Ingress for the app it is declared in. The app must have a
			port, or the route must set port. Without path or paths, the route
			uses /.

			Fields:
			  host         required  host name
			  path         optional  one path; defaults to /
			  paths        optional  several paths, as a list
			  port         optional  service port; defaults to the app's first port
			  tls          optional  Secret name for TLS
			  annotations  optional  merged over defaults.ingress_annotations
			  scope        optional  restrict the Ingress to one namespace

			ingress_class and ingress_annotations come from defaults.
			Without scope, the route renders wherever the app renders.
			Paths are Prefix type.
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

	envTopic = schemaTopic{
		name:    "env",
		summary: "Attach environment to a workload",
		body: `
			env {
			  LOG_LEVEL = "info"

			  config {
			    source = "./secrets/.env"
			    prefix = "CONFIG_"
			  }

			  secret {
			    source = "./secrets/.env"
			    prefix = "SECRET_"
			  }

			  from_config = config.shared
			  from_secret = secret.shared
			}

			Attaches environment variables to an app, stateful, job, or cron.

			Sources:
			  values       string attributes become container environment variables
			  config       creates <workload>-config from source, values, and files
			  secret       creates <workload>-secret from source and files
			  from_config  reuse a top-level config block
			  from_secret  reuse a top-level secret block

			Use from_config and from_secret when several workloads share one
			config or secret.

			POD_NAME, POD_NAMESPACE, POD_IP, POD_UID, NODE_NAME, and HOST_IP are
			always set from the pod. Do not declare them.
`,
	}

	scopeTopic = schemaTopic{
		name:    "scope",
		summary: "How a block chooses its namespace",
		body: `
			scope = "staging"

			Every block except defaults, volume, and manifest accepts scope.
			Namespaces are listed at the top level. A route also follows the
			app it is declared in.

			Without scope, the block renders into every declared namespace.
			With scope, the block renders only into that namespace.

			config and secret scope blocks override base values; a scoped file
			replaces the base file with the same name. appd run -n <namespace>
			renders one namespace at a time.
`,
	}
)

var schemaTopics = []schemaTopic{
	namespacesTopic,
	defaultsTopic,
	configTopic,
	secretTopic,
	volumeTopic,
	appTopic,
	statefulTopic,
	jobTopic,
	routeTopic,
	manifestTopic,
	envTopic,
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
