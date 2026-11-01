package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/ude-p/appdeploy/internal/config"
	"github.com/ude-p/appdeploy/internal/render"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}

	switch os.Args[1] {
	case "run":
		run(os.Args[2:])
	case "explain":
		explain(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fail(fmt.Errorf("unknown command %q", os.Args[1]))
	}
}

func run(args []string) {
	flags := flag.NewFlagSet("appd run", flag.ExitOnError)
	flags.Usage = func() {
		mustFprintln(flags.Output(), "Usage: appd run [-o | --kustomize dir] [-f deploy.hcl] [-n namespace] [--kubeconfig path] [--ctx name]")
		flags.PrintDefaults()
	}

	path := "deploy.hcl"
	namespace := ""
	output := false
	kustomize := ""
	context := ""
	kubeconfig := ""
	flags.StringVar(&path, "f", path, "path to the AppDeploy HCL file")
	flags.StringVar(&namespace, "n", namespace, "render one declared namespace")
	flags.BoolVar(&output, "o", false, "write Kubernetes YAML to stdout for inspection")
	flags.StringVar(&kustomize, "kustomize", kustomize, "write kustomized manifests with encrypted secrets to this directory")
	flags.StringVar(&context, "ctx", context, "Kubernetes context to use")
	flags.StringVar(&kubeconfig, "kubeconfig", kubeconfig, "path to kubeconfig file")
	if err := flags.Parse(args); err != nil {
		fail(err)
	}
	if flags.NArg() != 0 {
		fail(fmt.Errorf("unexpected arguments: %v", flags.Args()))
	}
	if output && kustomize != "" {
		fail(fmt.Errorf("cannot use -o with --kustomize"))
	}

	model, err := config.ParseHCL(path)
	if err != nil {
		fail(err)
	}
	if err := model.Validate(); err != nil {
		fail(fmt.Errorf("validate %q: %w", path, err))
	}

	namespaces := make([]string, 0, len(model.Namespaces))
	if namespace != "" {
		namespaces = append(namespaces, namespace)
	} else {
		namespaces = append(namespaces, model.Namespaces...)
	}

	if kustomize != "" {
		if err := render.Kustomize(model, kustomize, namespaces); err != nil {
			fail(err)
		}
		return
	}

	objects := make([]client.Object, 0)
	for _, namespace := range namespaces {
		resources, err := render.Build(model, namespace)
		if err != nil {
			fail(err)
		}
		objects = append(objects, resources...)
	}
	manifests, err := render.BuildManifests(model)
	if err != nil {
		fail(err)
	}
	objects = append(objects, manifests...)

	if output {
		objects, omitted := withoutSecrets(objects)
		if omitted > 0 {
			noun := "secrets"
			if omitted == 1 {
				noun = "secret"
			}
			mustFprintln(os.Stderr, fmt.Sprintf("appd: omitted %d %s; use appd run or --kustomize for full output", omitted, noun))
		}
		data, err := render.YAML(objects)
		if err != nil {
			fail(err)
		}
		if _, err := os.Stdout.Write(data); err != nil {
			fail(err)
		}
		return
	}

	if err := deploy(objects, kubeconfig, context); err != nil {
		fail(err)
	}
}

func withoutSecrets(objects []client.Object) ([]client.Object, int) {
	filtered := make([]client.Object, 0, len(objects))
	omitted := 0
	for _, object := range objects {
		if _, ok := object.(*corev1.Secret); ok {
			omitted++
			continue
		}
		filtered = append(filtered, object)
	}
	return filtered, omitted
}

func usage() {
	mustFprintln(os.Stderr, "Usage: appd <command>")
	mustFprintln(os.Stderr, "\nCommands:")
	mustFprintln(os.Stderr, "  explain list schema topics or describe one")
	mustFprintln(os.Stderr, "  run     build Kubernetes resources")
}

func fail(err error) {
	mustFprintln(os.Stderr, "appd:", err)
	os.Exit(1)
}

func mustFprintln(w io.Writer, args ...any) {
	if _, err := fmt.Fprintln(w, args...); err != nil {
		panic(err)
	}
}
