// Command terraform-provider-netcup serves the netcup provider to OpenTofu.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/quitordinary/terraform-provider-netcup/internal/provider"
)

// version is set at release time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	debug := flag.Bool("debug", false, "run with support for debuggers such as delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: provider.Address,
		Debug:   *debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
