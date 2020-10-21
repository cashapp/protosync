package resolver

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

// Local tries to resolve imports locally.
func Local(includes []string) Resolver {
	return func(path string) (NamedReadCloser, error) {
		for _, include := range includes {
			roots, err := filepath.Glob(include)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			for _, root := range roots {
				localPath := filepath.Join(root, path)
				r, err := os.Open(localPath)
				if os.IsNotExist(err) {
					continue
				} else if err != nil {
					return nil, errors.WithStack(err)
				}
				return r, nil
			}
		}
		return nil, nil
	}
}
