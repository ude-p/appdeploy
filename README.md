# AppDeploy

AppDeploy is a small Kubernetes deployment tool that reads an HCL file and turns it into Kubernetes resources. It is meant for teams that want a simple declarative deployment format for apps, stateful workloads, jobs, routes, secrets, config maps, volumes, and raw manifests.

## Install

```bash
go install github.com/ude-p/appdeploy/cmd/appd@latest
```

That installs the `appd` binary into your Go bin directory.

## Usage

Start by inspecting the schema:

```bash
appd explain
```

Then create a starter file:

```bash
appd init
```

And render or deploy your configuration with:

```bash
appd run
```

Use `appd run -o` to print the generated YAML instead of applying it to Kubernetes.
