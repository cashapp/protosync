// Package resolver contains the .proto resolver API and implementations.
package resolver

import (
	"io"
)

// NamedReadCloser gives an io.ReadCloser an identity.
type NamedReadCloser interface {
	Name() string
	io.ReadCloser
}

// A Resolver can resolve proto imports to source.
//
// Will return (nil, nil) if not found.
type Resolver func(path string) (NamedReadCloser, error)

// Combine a set of resolvers, trying each in turn.
func Combine(resolvers ...Resolver) Resolver {
	return func(path string) (NamedReadCloser, error) {
		for _, resolve := range resolvers {
			r, err := resolve(path)
			if err != nil {
				return nil, err
			}
			if r != nil {
				return r, nil
			}
		}
		return nil, nil
	}
}

type namedReadCloser struct {
	name string
	io.ReadCloser
}

func (n *namedReadCloser) Name() string { return n.name }
