# appdeploy

`appdeploy` is a Kubernetes operator for declaring an application stack from one custom resource.

## What it does

From a single `AppDeploy` resource, the controller can create:

- `ConfigMap` objects
- `ExternalSecret` objects for ESO
- `Deployment` objects
- `StatefulSet` objects with headless services
- `Job` objects for one-off workloads
- `Service` objects
- `Ingress` objects

It also wires pod fields like:

- `envFromConfig`
- `envFromSecrets`
- `imagePullSecrets`
- `imagePullPolicy`
- `resources`
- `volumeMounts`

## CRD Shape

Current API version: `appdeploy.io/v1`

Top-level spec fields:

- `namespaces`: required target namespaces
- `selectedNamespaces`: optional subset of `namespaces`
- `configMaps`: config maps to create
- `secrets`: ESO-backed secrets to fan out
- `workloads`: deployments, stateful sets, or jobs to create
- `ingresses`: ingress resources to create

### ConfigMaps

- `name`: required
- `scope`: optional
- `override`: optional, requires `scope`
- `data`: key/value data

An empty `scope` means the config map is created in every selected namespace.

Scoped config maps can override a default config map with the same name:

```yaml
configMaps:
  - name: app-config
    data:
      APP_ENV: prod
      HOST: 0.0.0.0
      PORT: "8090"

  - name: app-config
    scope: staging
    override: true
    data:
      APP_ENV: staging
```

The scoped override inherits default keys and only replaces the keys it defines.

### Secrets

- `name`: required
- `type`: `Opaque` or `ImagePull`
- `scope`: optional
- `remoteKey`: Vault path read by ESO
- `secretStoreName`: ESO store name, defaults to `cluster-vault`
- `secretStoreKind`: `SecretStore` or `ClusterSecretStore`, defaults to `ClusterSecretStore`

`ImagePull` secrets are rendered as `kubernetes.io/dockerconfigjson` secrets.

### Workloads

- `name`: required
- `kind`: `Deployment`, `StatefulSet`, or `Job`
- `scope`: optional
- `image`: required
- `replicas`: optional for workloads that run as deployments or stateful sets, defaults to `1`
- `containerPort`: required for deployments and stateful sets
- `command`: optional, useful for jobs
- `args`: optional, useful for jobs
- `backoffLimit`: optional, useful for jobs
- `ttlSecondsAfterFinished`: optional, useful for jobs
- `resources`: optional
- `imagePullPolicy`: optional
- `serviceType`: optional
- `servicePort`: optional, defaults to `containerPort`
- `headlessServiceName`: required for `StatefulSet`
- `envFromConfig`: optional list of ConfigMap names
- `envFromSecrets`: optional list of Secret names
- `imagePullSecrets`: optional list of Secret names
- `volumeMounts`: optional list of mounted ConfigMaps or Secrets
- `overrides`: raw patch blob for allowlisted fields not modeled in the typed schema

### Ingresses

- `name`: required
- `scope`: optional
- `className`: required
- `host`: required
- `annotations`: optional
- `tlsSecretName`: optional
- `rules`: path/service mappings
- `overrides`: raw patch blob for allowlisted fields not modeled in the typed schema

## Rules

- `namespaces` must contain at least one entry
- `selectedNamespaces` must be a subset of `namespaces`
- duplicate namespaces are rejected
- duplicate target object names in the same namespace are rejected
- empty `scope` means “apply to every selected namespace”
- config map overrides must be scoped
- config map overrides require a default config map with the same name
- config map overrides can only replace existing default keys
- `StatefulSet` workloads require `headlessServiceName`
- `Job` workloads skip service creation
- `volumeMounts` must specify exactly one of `configMapName` or `secretName`
- `overrides` may only use allowlisted fields and may not collide with schema-managed fields

## Workflow

Examples:

```sh
just manifests
just generate
just build
just run
just test
```

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0.
