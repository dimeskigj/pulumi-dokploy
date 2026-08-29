// Package main runs the provider's gRPC server.
package main

import (
	"context"
	"fmt"
	"os"

	dokploy "github.com/gjorgjidimeski/pulumi-dokploy/provider"
)

func main() {
	if err := dokploy.Provider().Run(context.Background(), dokploy.Name, dokploy.Version); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s", err.Error())
		os.Exit(1)
	}
}
