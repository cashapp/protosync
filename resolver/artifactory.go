package resolver

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/cashapp/protosync/log"
)

// ArtifactoryConfig defines how to talk to Artifactory and where to download artifacts from.
type ArtifactoryConfig struct {
	URL          string                        `hcl:"url" help:"Artifactory URL, eg. \"https://artifactory.mycompany.com/artifactory\""`
	DownloadURL  string                        `hcl:"download_url,optional" help:"Optional URL to download artifacts from. If not provided Artifactory itself will be used."`
	Repositories []ArtifactoryRepositoryConfig `hcl:"repository,block" help:"Artifactory repositories to download the latest JAR from."`
}

// ArtifactoryRepositoryConfig is the config for a single repository within Artifactory.
type ArtifactoryRepositoryConfig struct {
	Path    string `hcl:"name,label" help:"Artifact repository name."`
	Version string `hcl:"version,optional" help:"The artifact version to use."`
}

// ArtifactoryJAR resolves protobufs from JAR files in Artifactory.
//
// This will sync metadata from Artifactory, download, cache, and unpack the latest JAR.
//
// "artifactoryURL" should be the base Artifactory URL,
// eg. "https://artifactory.mycompany.com/artifactory".
// "jarURL" should have the same URL layout as Artifactory, but could be a JAR mirror,
// eg. "https://edge-cache.mycompany.com/artifactory".
// "repositoryPath" is the Artifactory repository path to the artifact we're retrieving,
// eg. "jar-releases/com/mycompany/external/protos/mycompany-protos" or "mycompany-public/com/mycompany/protos/all-protos"
func ArtifactoryJAR(artifactoryURL, jarURL string, repository ArtifactoryRepositoryConfig) Resolver {
	var jarPath string
	var zipFile *zip.ReadCloser
	return func(path string) (NamedReadCloser, error) {
		if zipFile == nil {
			var err error
			jarPath, zipFile, err = openJAR(artifactoryURL, jarURL, repository)
			if err != nil {
				return nil, err
			}
		}
		for _, file := range zipFile.File {
			if file.Name == path {
				r, err := file.Open()
				if err != nil {
					return nil, errors.Wrap(err, jarPath)
				}
				return &namedReadCloser{name: jarPath + "#" + path, ReadCloser: r}, nil
			}
		}
		return nil, nil
	}
}

// Download and cache latest version of a JAR file.
func openJAR(artifactoryURL, jarBaseURL string, repository ArtifactoryRepositoryConfig) (string, *zip.ReadCloser, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", nil, errors.WithStack(err)
	}

	artifactName := filepath.Base(repository.Path)
	version := repository.Version
	if version == "" {
		version, err = syncJARMetadata(artifactoryURL, repository.Path)
		if err != nil {
			return "", nil, err
		}
	}

	filename := fmt.Sprintf("%s-%s.jar", artifactName, version)
	dest := filepath.Join(cacheDir, filename)
	if _, err := os.Stat(dest); err == nil {
		zr, err := zip.OpenReader(dest)
		return dest, zr, errors.WithStack(err)
	}

	// Download the JAR file into the user's cache directory.
	jarPath := fmt.Sprintf("%s/%s/%s/%s", jarBaseURL, repository.Path, version, filename)
	log.Debugf("Syncing %s version %s", repository.Path, version)
	req, err := http.NewRequest("GET", jarPath, nil)
	if err != nil {
		return "", nil, errors.WithStack(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, errors.WithStack(err)
	}
	defer resp.Body.Close()

	log.Debugf("  <- %s (%s)", jarPath, humanSize(resp.ContentLength))
	log.Debugf("  -> %s", dest)
	w, err := ioutil.TempFile(cacheDir, artifactName+"-*.jar")
	if err != nil {
		return "", nil, errors.WithStack(err)
	}
	defer w.Close()

	_, err = io.Copy(w, resp.Body)
	if err != nil {
		return "", nil, errors.WithStack(err)
	}
	err = os.Rename(w.Name(), dest)
	if err != nil {
		return "", nil, errors.WithStack(err)
	}
	zr, err := zip.OpenReader(dest)
	return dest, zr, errors.WithStack(err)
}

// In any civilised world we'd just download the entire metadata file because it's simplest,
// but because Square's Artifactory is so MIND NUMBINGLY slow (+20s vs. 2s in Snapifact)
// we'll do a streaming read of the XML and abort as soon as we have the latest version.
func syncJARMetadata(artifactoryURL, repositoryPath string) (string, error) {
	log.Debugf("Syncing %s metadata.", repositoryPath)
	url := fmt.Sprintf("%s/%s/maven-metadata.xml", artifactoryURL, repositoryPath)
	resp, err := http.Get(url)
	if err != nil {
		return "", errors.WithStack(err)
	}
	log.Debugf("  <- %s (%s)", url, humanSize(resp.ContentLength))
	defer resp.Body.Close()
	dec := xml.NewDecoder(resp.Body)
	for {
		// Read tokens from the XML document in a stream.
		t, err := dec.Token()
		if err != nil {
			return "", errors.WithStack(err)
		}
		if t == nil {
			break
		}
		if se, ok := t.(xml.StartElement); ok {
			if se.Name.Local == "latest" {
				var version string
				return version, dec.DecodeElement(&version, &se)
			}
		}
	}
	return "", errors.Errorf("could not find latest version")
}

// nolint: gomnd
func humanSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%dB", n)
	case n < 1024*1024:
		return fmt.Sprintf("%dKiB", n/1024)
	default:
		return fmt.Sprintf("%dMiB", n/1024/1024)
	}
}
