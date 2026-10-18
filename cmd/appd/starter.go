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
