// Package protosync can sync .proto files from their source repositories to a local directory for importing.
package protosync

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"github.com/cashapp/protosync/log"
	"github.com/cashapp/protosync/parser"
	"github.com/cashapp/protosync/resolver"
)

// Sync a set of remote protobuf imports and/or recursively resolved local roots to dest.
//
// Returns the list of files synchronised into dest.
func Sync(resolve resolver.Resolver, dest string, sources ...string) ([]string, error) {
	roots := []string{}
	imports := []string{}
	for _, src := range sources {
		if strings.HasSuffix(src, ".proto") {
			imports = append(imports, src)
		} else {
			roots = append(roots, src)
		}
	}
	ctx := &context{
		dest:     dest,
		roots:    roots,
		resolved: map[string]bool{},
		resolve:  resolve,
	}
	for _, src := range imports {
		err := recursiveResolve(ctx, src)
		if err != nil {
			return nil, err
		}
	}
	for _, root := range roots {
		err := resolveLocalRoot(ctx, root)
		if err != nil {
			return nil, err
		}
	}
	synced := []string{}
	for imp := range ctx.resolved {
		synced = append(synced, imp)
	}
	return synced, nil
}

type context struct {
	roots    []string
	resolved map[string]bool
	resolve  resolver.Resolver
	dest     string
}

func resolveLocalRoot(ctx *context, root string) error {
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return errors.WithStack(err)
		}
		if !strings.HasSuffix(path, ".proto") {
			return nil
		}
		r, err := os.Open(path)
		if err != nil {
			return errors.WithStack(err)
		}
		defer r.Close()
		return resolveImports(ctx, r)
	})
	return errors.WithStack(err)
}

func resolveImports(ctx *context, r io.Reader) error {
	proto, err := parser.Parse(r)
	if err != nil {
		return errors.WithStack(err)
	}
	pkg := ""
nextImport:
	for _, stmt := range proto.Entries {
		if stmt.Package != "" {
			pkg = stmt.Package
			continue
		}
		if stmt.Import == "" {
			continue
		}
		// Skip local imports.
		for _, root := range ctx.roots {
			rootImport := filepath.Join(root, stmt.Import)
			if _, err := os.Stat(rootImport); err == nil {
				log.Tracef("%s imports %s (local %s)", pkg, stmt.Import, rootImport)
				continue nextImport
			}
		}
		if ctx.resolved[stmt.Import] {
			log.Tracef("%s imports %s (cached)", pkg, stmt.Import)
		} else {
			log.Tracef("%s imports %s (fetch)", pkg, stmt.Import)
		}
		err := recursiveResolve(ctx, stmt.Import)
		if err != nil {
			return errors.Wrap(err, stmt.Pos.String())
		}
	}
	return nil
}

func recursiveResolve(ctx *context, imp string) error {
	if ctx.resolved[imp] {
		return nil
	}
	r, err := ctx.resolve(imp)
	if err != nil {
		return errors.Wrapf(err, imp)
	}
	if r == nil {
		return errors.Errorf("could not resolve %q, may need resolver config to be updated", imp)
	}
	ctx.resolved[imp] = true
	defer r.Close()
	destFile := filepath.Join(ctx.dest, imp)
	err = os.MkdirAll(filepath.Dir(destFile), os.ModePerm)
	if err != nil {
		return errors.WithStack(err)
	}
	log.Infof("%s -> %s", r.Name(), destFile)
	w, err := os.Create(destFile)
	if err != nil {
		return errors.WithStack(err)
	}
	defer w.Close()
	_, err = io.Copy(w, r)
	if err != nil {
		return errors.WithStack(err)
	}
	_ = w.Close()
	_ = r.Close()

	// Recursively resolve imports.
	r, err = os.Open(destFile)
	if err != nil {
		return errors.WithStack(err)
	}
	defer r.Close()
	return resolveImports(ctx, r)
}
