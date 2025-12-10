package main

import (
	"os"

	"stormdragon/k8s-deployer/pkg/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}

