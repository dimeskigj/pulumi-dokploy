// Package main runs the provider's gRPC server.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gjorgjidimeski/pulumi-dokploy/provider"
)

func main() {
	if err := provider.Provider().Run(context.Background(), provider.Name, provider.Version); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s", err.Error())
		os.Exit(1)
	}
}
