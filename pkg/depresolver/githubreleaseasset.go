package depresolver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"

	"github.com/hashicorp/go-getter"
)

type githubReleaseAsset struct {
	client *getter.Client
}

var _ getter.Getter = &githubReleaseAsset{}
var _ getter.Detector = &githubReleaseAsset{}

var (
	downloadFileRegex = regexp.MustCompile("github.com/([^/]+)/([^/]+)/releases/download/([^/]+)/([^/]+)")
	downloadDirRegex  = regexp.MustCompile("github.com/([^/]+)/([^/]+)/releases/download/([^/]+)/?")
)

func (a *githubReleaseAsset) Detect(src string, _ string) (string, bool, error) {
	if len(src) == 0 {
		return "", false, nil
	}

	matches := downloadFileRegex.FindStringSubmatch(src)
	if len(matches) != 5 {
		return "", false, nil
	}

	// owner, repo, tag, assetName := matches[1], matches[2], matches[3], matches[4]

	urlStr := fmt.Sprintf("https://%s", matches[0])
	url, err := url.Parse(urlStr)
	if err != nil {
		return "", true, fmt.Errorf("error parsing GitHub URL: %s", err)
	}

	return "githubdownload::" + url.String(), true, nil
}

// ClientMode returns the mode based on the given URL. This is used to
// allow clients to let the getters decide which mode to use.
func (a *githubReleaseAsset) ClientMode(u *url.URL) (getter.ClientMode, error) {
	if downloadFileRegex.MatchString(u.String()) {
		return getter.ClientModeFile, nil
	}
	return getter.ClientModeDir, nil
}

// Get downloads the given URL into the given directory. This always
// assumes that we're updating and gets the latest version that it can.
//
// The directory may already exist (if we're updating). If it is in a
// format that isn't understood, an error should be returned. Get shouldn't
// simply nuke the directory.
func (a *githubReleaseAsset) Get(dst string, u *url.URL) error {
	matches := downloadDirRegex.FindStringSubmatch(u.String())
	if len(matches) != 4 {
		return nil
	}

	owner, repo, tag := matches[1], matches[2], matches[3]

	release, err := a.getReleaseByTag(owner, repo, tag)
	if err != nil {
		return err
	}

	assets, err := a.getAssetsByReleaseID(owner, repo, release.ID)
	if err != nil {
		return err
	}

	for _, asset := range assets {
		dstFile := filepath.Join(dst, asset.Name)
		if err := a.getFile(dstFile, owner, repo, asset.ID); err != nil {
			return err
		}
	}

	return nil
}

type Release struct {
	ID int64 `json:"id"`
}

type Asset struct {
	Name string `json:"name"`
	ID   int64  `json:"id"`
	URL  string `json:"url"`
}

type AssetsResponse struct {
	Assets []Asset
}

// GetFile downloads the give URL into the given path. The URL must
// reference a single file. If possible, the Getter should check if
// the remote end contains the same file and no-op this operation.
func (a *githubReleaseAsset) GetFile(dst string, src *url.URL) error {
	matches := downloadFileRegex.FindStringSubmatch(src.String())
	if len(matches) != 5 {
		return nil
	}

	owner, repo, tag, assetName := matches[1], matches[2], matches[3], matches[4]

	release, err := a.getReleaseByTag(owner, repo, tag)
	if err != nil {
		return err
	}

	assets, err := a.getAssetsByReleaseID(owner, repo, release.ID)
	if err != nil {
		return err
	}

	for _, asset := range assets {
		if asset.Name == assetName {
			if err := a.getFile(dst, owner, repo, asset.ID); err != nil {
				return err
			}

			break
		}
	}

	return nil
}

func (a *githubReleaseAsset) getReleaseByTag(owner, repo, tag string) (*Release, error) {
	var release Release

	reqURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", owner, repo, tag)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	if gt := os.Getenv("GITHUB_TOKEN"); gt != "" {
		req.Header = make(http.Header)
		req.Header.Add("authorization", "token "+gt)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", reqURL, res.Status)
	}

	d := json.NewDecoder(res.Body)

	if err := d.Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}

func (a *githubReleaseAsset) getAssetsByReleaseID(owner, repo string, releaseID int64) ([]Asset, error) {
	var assets []Asset

	reqURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/%d/assets", owner, repo, releaseID)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	if gt := os.Getenv("GITHUB_TOKEN"); gt != "" {
		req.Header = make(http.Header)
		req.Header.Add("authorization", "token "+gt)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", reqURL, res.Status)
	}

	d := json.NewDecoder(res.Body)

	if err := d.Decode(&assets); err != nil {
		return nil, err
	}

	return assets, nil
}

func (a *githubReleaseAsset) getFile(dst string, owner, repo string, assetID int64) error {
	// Create all the parent directories if needed
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	reqURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/assets/%d", owner, repo, assetID)
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return err
	}
	req.Header = make(http.Header)
	if gt := os.Getenv("GITHUB_TOKEN"); gt != "" {
		req.Header.Add("authorization", "token "+gt)
	}
	req.Header.Add("accept", "application/octet-stream")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", reqURL, res.Status)
	}

	f, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE, os.FileMode(0666))
	if err != nil {
		return fmt.Errorf("open file %s: %w", dst, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, res.Body); err != nil {
		return err
	}

	return nil
}

// SetClient allows a getter to know it's client
// in order to access client's Get functions or
// progress tracking.
func (a *githubReleaseAsset) SetClient(c *getter.Client) {
	a.client = c
}
