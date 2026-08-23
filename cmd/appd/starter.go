package main

const starterHCL = `
namespace "prod" {}
namespace "staging" {}

config "app" {
  env {
    source = ".env"
    prefix = "CONFIG_"
  }

  scope "staging" {
    APP_ENV = "staging"
  }
}

secret "app" {
  env {
    source = ".env"
    prefix = "SECRET_"
  }

  file "" {
    source = ""
  }

  scope "staging" {
    env {
      source = ".env"
      prefix = "SECRET_STAGING_"
    }
  }
}

app "api" {
  image    = ""
  replicas = 1

  env {
    config = config.app
    secret = secret.app
  }

  port {
    service = 80
  }

  resources {
    cpu    = "500m"
    memory = "512Mi"
  }

  health {
    path = "/health"
    port = 3000
  }
}

stateful "postgres" {
  image = "postgres:16"

  port {
    service = 5432
  }

  volume "data" {
    size          = "20Gi"
    storage_class = "standard"
    path          = "/var/lib/postgresql/data"
  }
}

job "migrate" {
  image   = ""
  command = []
  args    = []
  retries = 3
}

route "api" {
  to         = app.api
  host       = "api.demo.example"
  class_name = ""
  tls_secret = ""

  path "/" {}

  annotations = {
    "nginx.ingress.kubernetes.io/enable-cors" = "true"
  }
}
`

const schemaExplain = `
AppDeploy schema

Top-level blocks

namespace "name" {}
  Declares a namespace that AppDeploy can render into.

config "name" {
  env { source = ".env" prefix = "CONFIG_" }
  NEW_KEY = "value"
  data = { "file.txt" = "content" }
  file "name" { source = "./path" }
  scope "namespace" {
    EXISTING_KEY = "override"
  }
}
  Builds a ConfigMap from env files, new inline values, inline data, and optional files. Scope values can override existing values.

secret "name" {
  env { source = ".env" prefix = "SECRET_" }
  file "name" { source = "./path" }
  scope "namespace" { ... }
}
  Builds a Secret from env files, filtered by prefix, and optional files.

volume "name" {
  size = "10Gi"
  storage_class = "standard"
  access = "ReadWriteOnce"
}
  Creates a PersistentVolumeClaim that can be mounted by apps.

app "name" {
	  scope = ""
	  image = ""
	  replicas = 1
	  node_selector = { "workload" = "storage" }
  env {
    config = config.app
    secret = secret.app
    field "POD_NAME" { path = "metadata.name" }
  }
  port { service = 80 }
  resources { cpu = "500m" memory = "512Mi" }
  mount { volume = volume.data path = "/data" }
  mount { config = config.app path = "/config" }
  mount { secret = secret.app path = "/secrets" }
  health { path = "/health" port = 3000 }
}
  Creates a Deployment and optional Service.

stateful "name" {
	  scope = ""
	  image = "postgres:16"
	  node_selector = { "workload" = "storage" }
  env { ... }
  port { service = 5432 }
  volume "data" { size = "20Gi" path = "/var/lib/postgresql/data" }
  mount { config = config.init path = "/docker-entrypoint-initdb.d" }
}
  Creates a StatefulSet plus a headless Service.

job "name" {
	  scope = ""
	  image = ""
	  node_selector = { "workload" = "storage" }
  command = []
  args = []
  retries = 3
  env { ... }
  mount { volume = volume.data path = "/data" }
}
  Creates a one-shot Kubernetes Job.

cron "name" {
	  scope = ""
	  image = ""
	  node_selector = { "workload" = "storage" }
  schedule = "0 2 * * *"
  command = []
  args = []
  retries = 3
  env { ... }
}
  Creates a CronJob that runs on a schedule.

route "name" {
	  scope      = ""
	to         = app.api
	host       = "api.example.com"
	class_name = ""
	tls_secret = ""

	path "/" {}
	path "/metrics" {
	  port = 9090
	}
  annotations = { "key" = "value" }
}
  Creates an Ingress pointing at an app service. A path without port uses the app's first service port.

manifest "name" {
  yaml = <<YAML
apiVersion: example.com/v1
kind: Widget
metadata:
  name: widget
YAML
}
  Adds raw Kubernetes YAML for resources AppDeploy does not model natively.
`
