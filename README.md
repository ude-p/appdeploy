# appdeploy

`appdeploy` is a Kubernetes operator for declaring an application stack from one custom resource.

## What it does

From a single `AppDeploy` resource, the controller can create:

- `Namespace` objects
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

- `namespaces`: required namespaces to create and target
- `selectedNamespaces`: optional subset of `namespaces`
- `configMaps`: config maps to create
- `secrets`: ESO-backed secrets to fan out
- `persistentVolumeClaims`: persistent volume claims to create
- `workloads`: deployments, stateful sets, or jobs to create
- `ingresses`: ingress resources to create

Example:

```yaml
apiVersion: appdeploy.io/v1
kind: AppDeploy
metadata:
  name: demo-app
spec:
  namespaces: [prod, staging]
  selectedNamespaces: [staging]

  configMaps:
    - name: app-config
      data:
        APP_ENV: production
        PORT: "3000"

  secrets:
    - name: app-secret
      type: Opaque
      remoteKey: secrets/demo-app/app
      secretStoreName: cluster-vault
      secretStoreKind: ClusterSecretStore

  persistentVolumeClaims:
    - name: api-data
      accessModes: [ReadWriteOnce]
      storageClassName: standard
      resources:
        requests:
          storage: 10Gi

  workloads:
    - name: api
      kind: Deployment
      image: ghcr.io/acme/demo-api:v1
      replicas: 2
      serviceType: ClusterIP
      servicePorts: [80, 443]
      containerPorts: [3000, 3443]
      imagePullPolicy: IfNotPresent
      envFromConfig: [app-config]
      envFromSecrets: [app-secret]
      readinessProbe:
        httpGet:
          path: /ready
          port: 3000
        periodSeconds: 10
      livenessProbe:
        httpGet:
          path: /health
          port: 3000
        initialDelaySeconds: 15
        periodSeconds: 20
      volumeMounts:
        - name: api-data
          mountPath: /data
          persistentVolumeClaimName: api-data
      resources:
        requests:
          cpu: 100m
          memory: 128Mi
        limits:
          cpu: 500m
          memory: 512Mi

    - name: migrate
      kind: Job
      image: ghcr.io/acme/demo-api:v1
      args: ["npm", "run", "migrate"]
      envFromSecrets: [app-secret]

    - name: postgres
      kind: StatefulSet
      image: postgres:16
      replicas: 1
      serviceType: ClusterIP
      servicePorts: [5432]
      headlessServiceName: postgres
      volumeClaimTemplates:
        - name: postgres-data
          accessModes: [ReadWriteOnce]
          storageClassName: standard
          resources:
            requests:
              storage: 10Gi
      volumeMounts:
        - name: postgres-data
          mountPath: /var/lib/postgresql/data

  ingresses:
    - name: api
      className: traefik
      host: api.demo.example
      tlsSecretName: api-tls
      rules:
        - path: /
          serviceName: api
          servicePort: 80
```

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

### Persistent Volume Claims

- `name`: required
- `scope`: optional
- `accessModes`: required list, for example `ReadWriteOnce`
- `storageClassName`: optional
- `resources`: required Kubernetes volume resource requests, for example `requests.storage: 10Gi`

An empty `scope` means the PVC is created in every selected namespace.

Use top-level `persistentVolumeClaims` when a workload should mount a named PVC directly with `persistentVolumeClaimName`.

### Workloads

- `name`: required
- `kind`: `Deployment`, `StatefulSet`, or `Job`
- `scope`: optional
- `image`: required
- `replicas`: optional for workloads that run as deployments or stateful sets, defaults to `1`
- `servicePorts`: optional list of ports exposed by the Kubernetes Service for clients to call
- `containerPorts`: optional list of ports the application listens on inside the pod container; defaults to `servicePorts`
- `command`: optional, useful for jobs
- `args`: optional, useful for jobs
- `backoffLimit`: optional, useful for jobs
- `ttlSecondsAfterFinished`: optional, useful for jobs
- `resources`: optional
- `livenessProbe`: optional Kubernetes container liveness probe
- `readinessProbe`: optional Kubernetes container readiness probe
- `startupProbe`: optional Kubernetes container startup probe
- `imagePullPolicy`: optional
- `serviceType`: optional
- `headlessServiceName`: required for `StatefulSet`
- `envFromConfig`: optional list of ConfigMap names
- `envFromSecrets`: optional list of Secret names
- `imagePullSecrets`: optional list of Secret names
- `volumeMounts`: optional list of mounted ConfigMaps, Secrets, or PersistentVolumeClaims
- `volumeClaimTemplates`: optional list of StatefulSet PVC templates
- `overrides`: raw patch blob for allowlisted fields not modeled in the typed schema

`containerPort` and `servicePort` are no longer supported. Use `servicePorts` for the Service-facing ports, and set `containerPorts` only when the container listens on different ports:

```yaml
servicePorts: [80, 443]
containerPorts: [3000, 3443]
```

For StatefulSets, `volumeClaimTemplates` creates one PVC per pod. The `volumeMounts[].name` must match the template name:

```yaml
kind: StatefulSet
volumeClaimTemplates:
  - name: postgres-data
    accessModes: [ReadWriteOnce]
    resources:
      requests:
        storage: 10Gi
volumeMounts:
  - name: postgres-data
    mountPath: /var/lib/postgresql/data
```

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
- declared namespaces are created before namespace-scoped resources
- `selectedNamespaces` must be a subset of `namespaces`
- duplicate namespaces are rejected
- duplicate target object names in the same namespace are rejected
- empty `scope` means “apply to every selected namespace”
- config map overrides must be scoped
- config map overrides require a default config map with the same name
- config map overrides can only replace existing default keys
- `StatefulSet` workloads require `headlessServiceName`
- `volumeClaimTemplates` can only be used by `StatefulSet` workloads
- `Job` workloads skip service creation
- `volumeMounts` must specify exactly one of `configMapName`, `secretName`, or `persistentVolumeClaimName`, unless the mount name matches a `volumeClaimTemplates` name
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
