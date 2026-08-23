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
  file "name" { source = "./path" }
  scope "namespace" {
    EXISTING_KEY = "override"
  }
}
  Builds a ConfigMap from env files, new inline values, and optional files. Scope values can override existing values.

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
  image = ""
  replicas = 1
  env { config = config.app secret = secret.app }
  port { service = 80 }
  resources { cpu = "500m" memory = "512Mi" }
  mount { volume = volume.data path = "/data" }
  health { path = "/health" port = 3000 }
}
  Creates a Deployment and optional Service.

stateful "name" {
  image = "postgres:16"
  env { ... }
  port { service = 5432 }
  volume "data" { size = "20Gi" path = "/var/lib/postgresql/data" }
}
  Creates a StatefulSet plus a headless Service.

job "name" {
  image = ""
  command = []
  args = []
  retries = 3
  env { ... }
}
  Creates a one-shot Kubernetes Job.

cron "name" {
  image = ""
  schedule = "0 2 * * *"
  command = []
  args = []
  retries = 3
  env { ... }
}
  Creates a CronJob that runs on a schedule.

route "name" {
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
