// Package config contains the configuration and loader for protosync.
package config

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/alecthomas/hcl"
	"github.com/alecthomas/kong"
	"github.com/pkg/errors"

	"github.com/cashapp/protosync/resolver"
)

// Schema of the configuration.
var Schema = func() string {
	ast := hcl.MustSchema(&Config{})
	schema, _ := hcl.MarshalAST(ast)
	return string(schema)
}()

// Config represents the protosync index configuration format.
type Config struct {
	Dest        string                       `hcl:"dest,optional" help:"Destination where .proto files will be stored."`
	Remote      resolver.RemoteConfig        `hcl:"remote,block" help:"Configuration for remote repositories."`
	Sources     []string                     `hcl:"sources,optional" help:"List of remote imports or local root globals to resolve imports from."`
	Include     []string                     `hcl:"include,optional" help:"Globbed local include roots to search for proto files (eg. apps/*/protos)."`
	Artifactory []resolver.ArtifactoryConfig `hcl:"artifactory,block" help:"Retrieve protos from JAR files in Artifactory."`
	Repos       []resolver.Repo              `hcl:"repo,block" help:"Defines how to find protos in a source repository."`
}

func (c *Config) Decode(ctx *kong.DecodeContext) error { // nolint: golint
	value, err := ctx.Scan.PopValue("path")
	if err != nil {
		return errors.WithStack(err)
	}
	data, err := ioutil.ReadFile(value.Value.(string))
	if err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(hcl.Unmarshal(data, c))
}

// Resolve config to resolvers and glob-expanded sources.
func (c *Config) Resolve() (resolvers []resolver.Resolver, sources []string, err error) {
	resolvers = []resolver.Resolver{
		resolver.Local(c.Include),
		resolver.Remote(c.Remote, c.Repos),
	}
	for _, artifactory := range c.Artifactory {
		downloadURL := artifactory.DownloadURL
		if downloadURL == "" {
			downloadURL = artifactory.URL
		}
		for _, repo := range artifactory.Repositories {
			resolvers = append(resolvers, resolver.ArtifactoryJAR(artifactory.URL, downloadURL, repo))
		}
	}
	// Glob sources.
	for _, source := range c.Sources {
		matches, err := filepath.Glob(source)
		if err != nil {
			return nil, nil, errors.Wrap(err, source)
		}
		if len(matches) == 0 {
			sources = append(sources, source)
		} else {
			sources = append(sources, matches...)
		}
	}
	return
}

// Parse configuration.
//
// "vars" will be interpolated into the HCL before unmarshalling.
func Parse(config []byte, vars map[string]string) (*Config, error) {
	c := &Config{}
	ast, err := hcl.ParseBytes(config)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var substErr error
	subst := func(pos hcl.Position, s string) string {
		return os.Expand(s, func(key string) string {
			val, ok := vars[key]
			if !ok {
				substErr = errors.Errorf("%s: variable $%s not defined", pos, key)
				return "$" + key
			}
			return val
		})
	}
	err = hcl.Visit(ast, func(node hcl.Node, next func() error) error {
		switch node := node.(type) {
		case *hcl.Value:
			switch {
			case node.Str != nil:
				*node.Str = subst(node.Pos, *node.Str)
			case node.Heredoc != nil:
				*node.Heredoc = subst(node.Pos, *node.Heredoc)
			}
		}
		if substErr != nil {
			return substErr
		}
		return next()
	})
	if err != nil {
		return nil, err
	}
	return c, errors.WithStack(hcl.UnmarshalAST(ast, c))
}
