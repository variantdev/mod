package dockerregistry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTags_SinglePage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/myrepo/tags/list" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tagsResponse{Tags: []string{"v1.0.0", "v1.1.0", "v2.0.0"}})
	}))
	defer server.Close()

	client, err := New(server.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}

	tags, err := client.Tags("myrepo")
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{"v1.0.0", "v1.1.0", "v2.0.0"}
	if len(tags) != len(expected) {
		t.Errorf("expected %d tags, got %d", len(expected), len(tags))
	}
	for i, tag := range tags {
		if tag != expected[i] {
			t.Errorf("expected tag %d to be %s, got %s", i, expected[i], tag)
		}
	}
}

func TestTags_PaginationWithRelativeURL(t *testing.T) {
	page := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/myrepo/tags/list" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")

		if page == 0 {
			// First page: return relative URL in Link header (the bug we're fixing)
			w.Header().Set("Link", `</v2/myrepo/tags/list?last=v1.1.0>; rel="next"`)
			json.NewEncoder(w).Encode(tagsResponse{Tags: []string{"v1.0.0", "v1.1.0"}})
			page++
		} else {
			// Second page: no more pages
			json.NewEncoder(w).Encode(tagsResponse{Tags: []string{"v2.0.0", "v2.1.0"}})
		}
	}))
	defer server.Close()

	client, err := New(server.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}

	tags, err := client.Tags("myrepo")
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{"v1.0.0", "v1.1.0", "v2.0.0", "v2.1.0"}
	if len(tags) != len(expected) {
		t.Errorf("expected %d tags, got %d", len(expected), len(tags))
	}
	for i, tag := range tags {
		if tag != expected[i] {
			t.Errorf("expected tag %d to be %s, got %s", i, expected[i], tag)
		}
	}
}

func TestTags_PaginationWithAbsoluteURL(t *testing.T) {
	page := 0
	var serverURL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/myrepo/tags/list" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")

		if page == 0 {
			// First page: return absolute URL in Link header (legacy behavior)
			w.Header().Set("Link", `<`+serverURL+`/v2/myrepo/tags/list?last=v1.1.0>; rel="next"`)
			json.NewEncoder(w).Encode(tagsResponse{Tags: []string{"v1.0.0", "v1.1.0"}})
			page++
		} else {
			// Second page: no more pages
			json.NewEncoder(w).Encode(tagsResponse{Tags: []string{"v2.0.0", "v2.1.0"}})
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := New(server.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}

	tags, err := client.Tags("myrepo")
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{"v1.0.0", "v1.1.0", "v2.0.0", "v2.1.0"}
	if len(tags) != len(expected) {
		t.Errorf("expected %d tags, got %d", len(expected), len(tags))
	}
	for i, tag := range tags {
		if tag != expected[i] {
			t.Errorf("expected tag %d to be %s, got %s", i, expected[i], tag)
		}
	}
}

func TestTags_TokenAuth(t *testing.T) {
	var tokenAuthReceived string
	var bearerTokenReceived string
	var serverURL string
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Token endpoint
		if r.URL.Path == "/token" {
			tokenAuthReceived = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"token": "test-bearer-token"})
			return
		}

		// Registry endpoint
		requestCount++
		auth := r.Header.Get("Authorization")

		if requestCount == 1 && auth == "" {
			// First request: return 401 with WWW-Authenticate header
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+serverURL+`/token",service="registry.test",scope="repository:myrepo:pull"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Second request (retry) or if auth is present: check bearer token
		bearerTokenReceived = auth
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tagsResponse{Tags: []string{"v1.0.0"}})
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := New(server.URL, "testuser", "testpass")
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.Tags("myrepo")
	if err != nil {
		t.Fatal(err)
	}

	// Verify Basic Auth was sent to token endpoint
	expectedBasicAuth := "Basic dGVzdHVzZXI6dGVzdHBhc3M="
	if tokenAuthReceived != expectedBasicAuth {
		t.Errorf("expected token endpoint to receive Basic Auth %q, got %q", expectedBasicAuth, tokenAuthReceived)
	}

	// Verify Bearer token was sent to registry endpoint on retry
	expectedBearerAuth := "Bearer test-bearer-token"
	if bearerTokenReceived != expectedBearerAuth {
		t.Errorf("expected registry endpoint to receive Bearer token %q, got %q", expectedBearerAuth, bearerTokenReceived)
	}
}

func TestTags_NoAuthWhenCredentialsEmpty(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tagsResponse{Tags: []string{"v1.0.0"}})
	}))
	defer server.Close()

	client, err := New(server.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.Tags("myrepo")
	if err != nil {
		t.Fatal(err)
	}

	if receivedAuth != "" {
		t.Errorf("expected no Authorization header when credentials are empty, got %q", receivedAuth)
	}
}

func TestNew_InvalidURL(t *testing.T) {
	_, err := New("://invalid", "", "")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestTags_UnexpectedStatusCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client, err := New(server.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.Tags("myrepo")
	if err == nil {
		t.Error("expected error for unauthorized status")
	}
}
