package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/cashapp/protosync"
	"github.com/cashapp/protosync/config"
	"github.com/cashapp/protosync/log"
	"github.com/cashapp/protosync/resolver"
)

const (
	builtinConfig = `
repo "https://github.com/protocolbuffers/protobuf.git" {
  prefix = "google/protobuf/"
  root = "src"
}

repo "https://github.com/googleapis/googleapis.git" {
  prefix = "google/"
}

repo "https://github.com/grpc-ecosystem/grpc-gateway.git" {
  prefix = "protoc-gen-swagger/"
  commit = "v1.15.2"
}
`
	help = `
This tool syncs the transitive import closure of a set of .proto files to a
local directory. A configuration file tells protosync where to look for .proto
files. It then retrieves and parses the .proto files specified on the
command-line, recursively retrieving all imports.

Configuration format:

%s

Default repositories always included (unless --no-defaults is used):

%s

`
)

var cli struct {
	LoggingConfig log.Config     `embed:""`
	Config        *config.Config `help:"Protosync config file path." placeholder:"protosync.hcl"`
	Dest          string         `short:"d" type:"existingdir" placeholder:"DIR" help:"Destination root to sync files to."`
	Includes      []string       `short:"I" help:"Additional local include roots to search, and scan for dependencies to resolve."`
	Sources       []string       `arg:"" optional:"" help:"Additional proto files to sync."`
	NoDefaults    bool           `help:"Don't include the set of default repositories.'"`
}

func main() {
	ctx := kong.Parse(&cli, kong.UsageOnError(), kong.Description(fmt.Sprintf(help, indent(config.Schema), indent(builtinConfig))))
	if cli.Config == nil {
		if cli.NoDefaults {
			cli.Config = &config.Config{}
		} else if r, err := os.Open("protosync.hcl"); err == nil {
			data, err := ioutil.ReadAll(r)
			_ = r.Close()
			ctx.FatalIfErrorf(err)
			conf, err := config.Parse(data)
			ctx.FatalIfErrorf(err)
			cli.Config = conf
		} else if os.IsNotExist(err) {
			conf, err := config.Parse([]byte(builtinConfig))
			ctx.FatalIfErrorf(err)
			cli.Config = conf
		} else {
			ctx.FatalIfErrorf(err)
		}
	}
	dest := cli.Dest
	if dest == "" {
		dest = cli.Config.Dest
	}
	if dest == "" {
		ctx.Fatalf("destination not provided on command line (--dest) or configuration file")
	}
	err := log.Configure(cli.LoggingConfig)
	ctx.FatalIfErrorf(err)
	resolvers, sources, err := cli.Config.Resolve()
	ctx.FatalIfErrorf(err)
	resolvers = append(resolvers, resolver.Local(cli.Includes))
	sources = append(sources, cli.Sources...)
	sources = append(sources, cli.Includes...)
	if len(sources) == 0 {
		ctx.PrintUsage(false) // nolint: errcheck
		fmt.Println()
		ctx.Fatalf("sources not provided on command line (--sources) or configuration file")
	}
	_, err = protosync.Sync(resolver.Combine(resolvers...), dest, sources...)
	ctx.FatalIfErrorf(err)
}

func indent(s string) string {
	return "\n  " + strings.Join(strings.Split(strings.TrimSpace(s), "\n"), "\n  ")
}
