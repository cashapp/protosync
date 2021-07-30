package resolver // nolint: testpackage

import (
	"net/url"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestGithubFetcherShouldNotChangeURLScheme(t *testing.T) {
	t.Parallel()
	ru := "ssh://git@github.com/cashapp/protosync.git"
	u, err := url.Parse(ru)
	require.NoError(t, err)

	reader, err := githubFetcher(u, "nonexistingcontent", "")
	require.True(t, errors.Is(err, errNotFound))
	require.Nil(t, reader)

	require.NotEqual(t, "https", u.Scheme)
	require.Equal(t, ru, u.String())
}
