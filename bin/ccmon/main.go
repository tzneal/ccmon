package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"k8s.io/client-go/util/homedir"

	"github.com/tzneal/ccmon/scenario"
)

func main() {

	fs := flag.NewFlagSet("ccom", flag.ExitOnError)
	kubeConfig := fs.String("kubeconfig", filepath.Join(homedir.HomeDir(), ".kube", "config"),
		"absolute path to the kubeconfig file")
	fs.Parse(os.Args[1:])

	if fs.NArg() == 0 {
		fs.Usage()
		os.Exit(1)
	}

	// open our scenario
	scenarioFile := fs.Arg(0)
	f, err := os.Open(scenarioFile)
	if err != nil {
		log.Fatalf("opening %s, %s", scenarioFile, err)
	}
	defer f.Close()
	scen, err := scenario.Open(f)
	if err != nil {
		log.Fatalf("reading %s, %s", scenarioFile, err)
	}
	fmt.Println(scen)

	runner, err := scenario.NewRunner(*kubeConfig)
	if err != nil {
		log.Fatalf("creating scenario runner, %s", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := runner.Execute(ctx, scen); err != nil {
		log.Fatalf("executing scenario, %s", err)
	}
}
