// Package dockerregistry provides a minimal Docker Registry API v2 client.
//
// This package exists because the heroku/docker-registry-client library is
// broken when interacting with Docker Hub. Docker Hub now returns relative URL
// paths in pagination Link headers instead of full URLs. This causes the
// heroku library to fail with "unsupported protocol schema """ when fetching
// tags that span multiple pages, because the request URL for the second page
// lacks the protocol and host part (since the next link is relative).
//
// This client properly handles relative URLs by resolving them against the
// base registry URL.
//
// Some code in this package is derived from heroku/docker-registry-client:
// https://github.com/heroku/docker-registry-client
// See transport.go for specific attributions.
package dockerregistry

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
)

var (
	// ErrNoMorePages is returned when there are no more pages to fetch.
	ErrNoMorePages = errors.New("no more pages")
)

// Client handles Docker Registry API v2 requests.
type Client struct {
	baseURL  *url.URL
	username string
	password string
	client   *http.Client
}

// Option is a functional option for configuring the Client.
type Option func(*Client)

// WithHTTPClient sets a custom HTTP client for the registry client.
// This is useful for testing or when custom transport settings are needed.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		c.client = client
	}
}

// New creates a new registry client for the given base URL.
// If username and password are non-empty, they will be used for token authentication.
func New(baseURL, username, password string, opts ...Option) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	c := &Client{
		baseURL:  u,
		username: username,
		password: password,
		client:   nil, // will be set below or by options
	}
	for _, opt := range opts {
		opt(c)
	}

	// Wrap the transport with token auth support
	if c.client == nil {
		c.client = &http.Client{
			Transport: WrapTransport(http.DefaultTransport, username, password),
		}
	} else {
		// Wrap the custom client's transport with token auth
		transport := c.client.Transport
		if transport == nil {
			transport = http.DefaultTransport
		}
		c.client = &http.Client{
			Transport: WrapTransport(transport, username, password),
		}
	}
	return c, nil
}

type tagsResponse struct {
	Tags []string `json:"tags"`
}

// Tags fetches all tags for a repository, handling pagination.
func (c *Client) Tags(repository string) ([]string, error) {
	u := c.url("/v2/%s/tags/list", repository)

	var tags []string
	for {
		var response tagsResponse
		nextURL, err := c.getPaginatedJSON(u, &response)
		switch err {
		case ErrNoMorePages:
			tags = append(tags, response.Tags...)
			return tags, nil
		case nil:
			tags = append(tags, response.Tags...)
			u = nextURL
			continue
		default:
			return nil, err
		}
	}
}

// url constructs a full URL from the base URL and the given path format.
func (c *Client) url(pathFormat string, args ...interface{}) string {
	path := fmt.Sprintf(pathFormat, args...)
	u := *c.baseURL
	u.Path = path
	return u.String()
}

// getPaginatedJSON fetches a URL and decodes the JSON response into the given
// interface. It returns the next page URL if present in the Link header,
// or ErrNoMorePages if there are no more pages.
func (c *Client) getPaginatedJSON(urlStr string, response interface{}) (string, error) {
	req, err := http.NewRequest(http.MethodGet, urlStr, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(response); err != nil {
		return "", err
	}

	return c.getNextLink(resp, urlStr)
}

// nextLinkRE matches an RFC 5988 (https://tools.ietf.org/html/rfc5988#section-5)
// Link header. For example,
//
//	<http://registry.example.com/v2/_catalog?n=5&last=tag5>; type="application/json"; rel="next"
//
// The URL is _supposed_ to be wrapped by angle brackets `< ... >`,
// but e.g., quay.io does not include them. Similarly, params like
// `rel="next"` may not have quoted values in the wild.
//
// Derived from: https://github.com/heroku/docker-registry-client/blob/master/registry/json.go
var nextLinkRE = regexp.MustCompile(`^ *<?([^;>]+)>? *(?:;[^;]*)*; *rel="?next"?(?:;.*)?`)

// getNextLink extracts the next page URL from the Link header.
// It handles both absolute and relative URLs by resolving against the current request URL.
func (c *Client) getNextLink(resp *http.Response, currentURL string) (string, error) {
	for _, link := range resp.Header[http.CanonicalHeaderKey("Link")] {
		parts := nextLinkRE.FindStringSubmatch(link)
		if parts != nil {
			nextURL := parts[1]

			// Parse the next URL to check if it's relative or absolute
			parsedNext, err := url.Parse(nextURL)
			if err != nil {
				return "", fmt.Errorf("invalid next link URL: %w", err)
			}

			// If the URL is relative (no scheme), resolve it against the current URL
			if parsedNext.Scheme == "" {
				parsedCurrent, err := url.Parse(currentURL)
				if err != nil {
					return "", fmt.Errorf("invalid current URL: %w", err)
				}
				resolved := parsedCurrent.ResolveReference(parsedNext)
				return resolved.String(), nil
			}

			return nextURL, nil
		}
	}
	return "", ErrNoMorePages
}
