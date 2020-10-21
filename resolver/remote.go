package resolver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// Repo defines a source repository and where to retrieve protos from it.
type Repo struct {
	URL        string   `hcl:"url,label" help:"Git cloneable URL of repository."`
	Root       string   `hcl:"root,optional" help:"Root path in remote repository to search for protos."`
	Prefix     string   `hcl:"prefix,optional" help:"Prefix of proto path that will match this repository. eg. 'google'"`
	Protos     []string `hcl:"protos,optional" help:"A list of specific .proto files that this repository contains."`
	CommitHash string   `hcl:"commit,optional" help:"Specific commit to retrieve .proto files from."`
}

// Commit from which to retrieve protos.
func (r *Repo) Commit() string {
	if r.CommitHash == "" {
		return "master"
	}
	return r.CommitHash
}

// RemoteConfig contains the configuration for Remote().
type RemoteConfig struct {
	BitbucketServers []string `hcl:"bitbucket-servers,optional" help:"List of hostnames to treat as Bitbucket servers."`
}

// Remote resolves imports from their source repositories.
func Remote(config RemoteConfig, repos []Repo) Resolver {
	return func(path string) (NamedReadCloser, error) {
		repo := findRepoForImport(repos, path)
		if repo == nil {
			return nil, nil
		}
		return fetchProto(config, repo, path)
	}
}

func findRepoForImport(repos []Repo, path string) *Repo {
	for _, repo := range repos {
		if repo.Prefix != "" && strings.HasPrefix(path, repo.Prefix) {
			return &repo
		}
		for _, proto := range repo.Protos {
			if proto == path {
				return &repo
			}
		}
	}
	return nil
}

type fetcherFunc func(u *url.URL, src, commit string) (NamedReadCloser, error)

func fetchProto(config RemoteConfig, repo *Repo, proto string) (NamedReadCloser, error) {
	repoURL, err := url.Parse(repo.URL)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	fetcher, err := chooseFetcher(config, repo, repoURL)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	u := &url.URL{}
	*u = *repoURL
	relPath := path.Join(repo.Root, proto)
	r, err := fetcher(u, relPath, repo.Commit())
	if errors.Is(err, errNotFound) { // try cloning repo
		fetchErr := err
		if r, err = cloner(u, relPath, repo.Commit()); err != nil {
			err = errors.Wrap(fetchErr, err.Error())
		}
	}
	if err != nil {
		return nil, errors.Wrap(err, repo.URL)
	}

	return r, nil
}

func chooseFetcher(config RemoteConfig, repo *Repo, repoURL *url.URL) (fetcherFunc, error) {
	if repoURL.Host == "github.com" {
		return githubFetcher, nil
	}
	for _, bitbucket := range config.BitbucketServers {
		if repoURL.Host == bitbucket {
			return bitBucketFetcher, nil
		}
	}
	return nil, errors.Errorf("unsupported repository source %q", repo.URL)
}

func bitBucketFetcher(repoURL *url.URL, relSrc, commit string) (NamedReadCloser, error) {
	u := &url.URL{}
	*u = *repoURL
	// Override ssh+git
	u.Scheme = "https"
	u.User = nil
	// eg. /scm/myompany/myservice.git
	parts := strings.Split(strings.TrimSuffix(u.Path, ".git"), "/")
	if len(parts) != 4 || parts[1] != "scm" {
		return nil, errors.Errorf("expected Bitbucket URL path in the form /scm/<project>/<repo>.git but got %q", u.Path)
	}
	project := parts[2]
	repo := parts[3]
	u.Path = path.Join("projects", project, "repos", repo, "raw", relSrc)
	u.RawQuery = "at=" + commit
	return httpGet(u.String())
}

func githubFetcher(u *url.URL, relSrc, commit string) (NamedReadCloser, error) {
	u.Scheme = "https"
	parts := strings.Split(strings.TrimSuffix(u.Path, ".git"), "/")
	if len(parts) != 3 {
		return nil, errors.Errorf("expected GitHub URL path in the form /<user>/<repo>.git but got %q", u.Path)
	}
	user := parts[1]
	project := parts[2]
	u, err := url.Parse(fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", user, project, commit, relSrc))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return httpGet(u.String())
}

var errNotFound = errors.New("not found")

func httpGet(srcURL string) (NamedReadCloser, error) {
	resp, err := http.Get(srcURL)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		s := &strings.Builder{}
		_, _ = io.Copy(s, resp.Body)
		resp.Body.Close()
		return nil, errors.Wrap(errNotFound, s.String())
	}
	if contentType := resp.Header.Get("Content-Type"); strings.HasPrefix(contentType, "text/html") {
		resp.Body.Close()
		return nil, errors.WithStack(errNotFound)
	}
	return &namedReadCloser{name: srcURL, ReadCloser: resp.Body}, nil
}

// cloner is a fetcherFunc that git-clones repo to user-cache directory
// and reads file. It is used when direct http download fails, for
// instance because of permission issues.
func cloner(u *url.URL, relPath, commit string) (NamedReadCloser, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	repo := filepath.Base(u.Path) + "-" + hash(u.String(), commit)
	dest := path.Join(cacheDir, "protosync", repo)
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return nil, errors.Wrapf(err, "cannot create protosync cache directory %q", dest)
	}
	if err := gitClone(u.String(), dest); err != nil {
		return nil, errors.WithStack(err)
	}
	if err := runInDir(dest, "git", "checkout", commit); err != nil {
		return nil, errors.WithStack(err)
	}
	name := fmt.Sprintf("%s + %s", u.String(), relPath)
	r, err := os.Open(path.Join(dest, relPath))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &namedReadCloser{name: name, ReadCloser: r}, nil
}

func gitClone(sourceURL, destDir string) error {
	// First, if a git repo exists, just pull.
	info, _ := os.Stat(path.Join(destDir, ".git"))
	if info != nil {
		return runInDir(destDir, "git", "pull")
	}
	// No git repo, clone down to temporary directory.
	tmpDestDir, err := os.MkdirTemp(filepath.Dir(destDir), filepath.Base(destDir)+"-*")
	if err != nil {
		return errors.Wrap(err, "cannot create temp directory for git clone")
	}
	defer os.RemoveAll(tmpDestDir)
	if err = runInDir(tmpDestDir, "git", "clone", sourceURL, tmpDestDir); err != nil {
		return errors.WithStack(err)
	}
	// And finally, rename it into place.
	if err = os.Rename(tmpDestDir, destDir); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// runInDir runs a command in the given directory.
func runInDir(dir, cmdStr string, args ...string) error {
	cmd := exec.Command(cmdStr, args...)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "%s %s failed", cmd, strings.Join(args, " "))
	}
	return nil
}

func hash(values ...interface{}) string {
	w := sha256.New()
	enc := json.NewEncoder(w)
	for _, value := range values {
		_ = enc.Encode(value)
	}
	return hex.EncodeToString(w.Sum(nil))
}
