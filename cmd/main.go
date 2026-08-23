package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ude-p/appdeploy/internal/config"
	"github.com/ude-p/appdeploy/internal/render"
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
	case "init":
		initialize(os.Args[2:])
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
		mustFprintln(flags.Output(), "Usage: appd run [-o] [-f deploy.hcl] [-n namespace] [--kubeconfig path] [--ctx name]")
		flags.PrintDefaults()
	}

	path := "deploy.hcl"
	namespace := ""
	output := false
	context := ""
	kubeconfig := ""
	flags.StringVar(&path, "f", path, "path to the AppDeploy HCL file")
	flags.StringVar(&path, "file", path, "path to the AppDeploy HCL file")
	flags.StringVar(&namespace, "n", namespace, "render one declared namespace")
	flags.StringVar(&namespace, "namespace", namespace, "render one declared namespace")
	flags.BoolVar(&output, "o", false, "write Kubernetes YAML to stdout for inspection")
	flags.BoolVar(&output, "output", false, "write Kubernetes YAML to stdout for inspection")
	flags.StringVar(&context, "ctx", context, "Kubernetes context to use")
	flags.StringVar(&context, "context", context, "Kubernetes context to use")
	flags.StringVar(&kubeconfig, "kubeconfig", kubeconfig, "path to kubeconfig file")
	if err := flags.Parse(args); err != nil {
		fail(err)
	}
	if flags.NArg() != 0 {
		fail(fmt.Errorf("unexpected arguments: %v", flags.Args()))
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
		for _, item := range model.Namespaces {
			namespaces = append(namespaces, item.Name)
		}
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

func initialize(args []string) {
	if len(args) != 0 {
		fail(fmt.Errorf("init does not accept arguments"))
	}

	file, err := os.OpenFile("deploy.hcl", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		fail(fmt.Errorf("create deploy.hcl: %w", err))
	}
	defer func() {
		if err := file.Close(); err != nil {
			fail(fmt.Errorf("close deploy.hcl: %w", err))
		}
	}()

	if _, err := file.WriteString(starterHCL); err != nil {
		fail(fmt.Errorf("write deploy.hcl: %w", err))
	}
}

func explain(args []string) {
	if len(args) != 0 {
		fail(fmt.Errorf("explain does not accept arguments"))
	}
	mustFprintln(os.Stdout, strings.TrimSpace(schemaExplain))
}

func usage() {
	mustFprintln(os.Stderr, "Usage: appd <command>")
	mustFprintln(os.Stderr, "\nCommands:")
	mustFprintln(os.Stderr, "  init    create deploy.hcl in the current directory")
	mustFprintln(os.Stderr, "  explain describe the AppDeploy schema")
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
