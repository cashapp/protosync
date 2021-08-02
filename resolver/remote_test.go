package resolver // nolint: testpackage

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestGithubFetcherShouldNotChangeURLScheme(t *testing.T) {
	t.Parallel()
	ru := "ssh://git@github.com/cashapp/protosync.git"
	repoWithShortURL := &Repo{
		URL: ru,
	}
	u, err := repoWithShortURL.ParseURL()
	require.NoError(t, err)

	reader, err := githubFetcher(u, "nonexistingcontent", "")
	require.True(t, errors.Is(err, errNotFound))
	require.Nil(t, reader)

	require.NotEqual(t, "https", u.Scheme)
	require.Equal(t, ru, u.String())
}

func TestGithubFetcherShouldBeOkWithDifferentURLs(t *testing.T) {
	t.Parallel()
	urls := []string{
		"git@github.com:cashapp/protosync.git",
		"https://ithub.com/cashapp/protosync.git",
		"ssh://git@github.com/cashapp/protosync.git",
	}

	for _, us := range urls {
		repoWithShortURL := &Repo{
			URL: us,
		}
		u, err := repoWithShortURL.ParseURL()
		require.NoError(t, err)

		reader, err := githubFetcher(u, "nonexistingcontent", "")
		require.True(t, errors.Is(err, errNotFound))
		require.Nil(t, reader)
	}
}

func TestRepoSSHShortURLParsing(t *testing.T) {
	t.Parallel()
	repoWithShortURL := &Repo{
		URL: "git-1234@github.com:cashapp/protosync.git",
	}
	u, err := repoWithShortURL.ParseURL()
	require.NoError(t, err)
	require.Equal(t, "ssh", u.Scheme)
	require.Equal(t, "github.com", u.Host)
	require.Equal(t, "cashapp/protosync.git", u.Path)
	require.Equal(t, "git-1234", u.User.Username())
}
