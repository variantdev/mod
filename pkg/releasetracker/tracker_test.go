package releasetracker

import (
	"github.com/variantdev/mod/pkg/cmdsite"
	"github.com/variantdev/mod/pkg/vhttpget"
	"gopkg.in/yaml.v3"
	"testing"
)

func TestProvider_JSONPath(t *testing.T) {
	input := `releaseChannel:
  versionsFrom:
    jsonPath:
      source: https://coreos.com/releases/releases-stable.json
      versions: "$"
      type: semver
      description: "$['{{.version}}'].release_notes"
`

	conf := &Config{}
	if err := yaml.Unmarshal([]byte(input), conf); err != nil {
		t.Fatal(err)
	}

	stable, err := New(conf.ReleaseChannel)
	if err != nil {
		t.Fatal(err)
	}

	latest, err := stable.Latest("= 2079.5.1")
	if err != nil {
		t.Fatal(err)
	}

	if latest.Version != "2079.5.1" {
		t.Errorf("unexpected version: expected=%v, got=%v", "2079.5.1", latest.Version)
	}
}

func TestProvider_Exec(t *testing.T) {
	input := `releaseChannel:
  versionsFrom:
    exec:
      command: sh
      args:
      - -c
      - cd ../../examples/eks-k8s-vers && go run main.go
`

	conf := &Config{}
	if err := yaml.Unmarshal([]byte(input), conf); err != nil {
		t.Fatal(err)
	}

	expectedInput := cmdsite.NewInput("sh", []string{"-c", "cd ../../examples/eks-k8s-vers && go run main.go"}, map[string]string{})
	expectedStdout := `1.13.7
1.12.6
1.11.8
1.10.13
`
	cmdr := cmdsite.NewTester(map[cmdsite.CommandInput]cmdsite.CommandOutput{
		expectedInput: {Stdout: expectedStdout},
	})

	stable, err := New(conf.ReleaseChannel, Commander(cmdr))
	if err != nil {
		t.Fatal(err)
	}

	latest, err := stable.Latest("= 1.13.7")
	if err != nil {
		t.Fatal(err)
	}

	if latest.Version != "1.13.7" {
		t.Errorf("unexpected version: expected=%v, got=%v", "1.13.7", latest.Version)
	}
}

func TestProvider_GitTags(t *testing.T) {
	input := `releaseChannel:
  versionsFrom:
    # This basically runs "git ls-remote --tags git://github.com/mumoshu/variant.git" to fetch available versions
    # Examples can be obtained by running:
    #   git ls-remote --tags git://github.com/mumoshu/variant.git | grep -v { | awk '{ print $2 }' | cut -d'/' -f 3
    gitTags:
      source: github.com/mumoshu/variant
`

	conf := &Config{}
	if err := yaml.Unmarshal([]byte(input), conf); err != nil {
		t.Fatal(err)
	}

	expectedStdout := `v0.21.2
v0.22.0
v0.23.0
v0.24.0
v0.24.1
v0.25.0
v0.25.1
v0.25.2
v0.26.0
v0.27.0
v0.27.1
v0.27.2
v0.27.3
v0.27.4
v0.27.5
v0.28.0
v0.29.0
v0.30.0
v0.31.0
v0.31.1
`

	expectedInput := cmdsite.NewInput("sh", []string{"-c", "git ls-remote --tags git://github.com/mumoshu/variant.git | grep -v { | awk '{ print $2 }' | cut -d'/' -f 3"}, map[string]string{})
	cmdr := cmdsite.NewTester(map[cmdsite.CommandInput]cmdsite.CommandOutput{
		expectedInput: {Stdout: expectedStdout},
	})
	stable, err := New(conf.ReleaseChannel, Commander(cmdr))
	if err != nil {
		t.Fatal(err)
	}

	latest, err := stable.Latest("= 0.31.1")
	if err != nil {
		t.Fatal(err)
	}

	expected := "0.31.1"
	if latest.Version != expected {
		t.Errorf("unexpected version: expected=%v, got=%v", expected, latest.Version)
	}
}

func TestProvider_GitHubReleases(t *testing.T) {
	input := `releaseChannel:
  versionsFrom:
    # This basically fetch "curl https://api.github.com/repos/mumoshu/variant/releases | jq -r .[].tag_name"
    githubReleases:
      source: mumoshu/variant
`

	conf := &Config{}
	if err := yaml.Unmarshal([]byte(input), conf); err != nil {
		t.Fatal(err)
	}

	expectedOut := `[
  {
    "url": "https://api.github.com/repos/mumoshu/variant/releases/18205178",
    "assets_url": "https://api.github.com/repos/mumoshu/variant/releases/18205178/assets",
    "upload_url": "https://uploads.github.com/repos/mumoshu/variant/releases/18205178/assets{?name,label}",
    "html_url": "https://github.com/mumoshu/variant/releases/tag/v0.31.1",
    "id": 18205178,
    "node_id": "MDc6UmVsZWFzZTE4MjA1MTc4",
    "tag_name": "v0.31.1",
    "target_commitish": "master",
    "name": "v0.31.1",
    "draft": false,
    "author": {
      "login": "mumoshu",
      "id": 22009,
      "node_id": "MDQ6VXNlcjIyMDA5",
      "avatar_url": "https://avatars0.githubusercontent.com/u/22009?v=4",
      "gravatar_id": "",
      "url": "https://api.github.com/users/mumoshu",
      "html_url": "https://github.com/mumoshu",
      "followers_url": "https://api.github.com/users/mumoshu/followers",
      "following_url": "https://api.github.com/users/mumoshu/following{/other_user}",
      "gists_url": "https://api.github.com/users/mumoshu/gists{/gist_id}",
      "starred_url": "https://api.github.com/users/mumoshu/starred{/owner}{/repo}",
      "subscriptions_url": "https://api.github.com/users/mumoshu/subscriptions",
      "organizations_url": "https://api.github.com/users/mumoshu/orgs",
      "repos_url": "https://api.github.com/users/mumoshu/repos",
      "events_url": "https://api.github.com/users/mumoshu/events{/privacy}",
      "received_events_url": "https://api.github.com/users/mumoshu/received_events",
      "type": "User",
      "site_admin": false
    },
    "prerelease": false,
    "created_at": "2019-06-25T10:38:52Z",
    "published_at": "2019-06-25T10:40:43Z",
    "assets": [
      {
        "url": "https://api.github.com/repos/mumoshu/variant/releases/assets/13388587",
        "id": 13388587,
        "node_id": "MDEyOlJlbGVhc2VBc3NldDEzMzg4NTg3",
        "name": "variant_0.31.1_checksums.txt",
        "label": "",
        "uploader": {
          "login": "mumoshu",
          "id": 22009,
          "node_id": "MDQ6VXNlcjIyMDA5",
          "avatar_url": "https://avatars0.githubusercontent.com/u/22009?v=4",
          "gravatar_id": "",
          "url": "https://api.github.com/users/mumoshu",
          "html_url": "https://github.com/mumoshu",
          "followers_url": "https://api.github.com/users/mumoshu/followers",
          "following_url": "https://api.github.com/users/mumoshu/following{/other_user}",
          "gists_url": "https://api.github.com/users/mumoshu/gists{/gist_id}",
          "starred_url": "https://api.github.com/users/mumoshu/starred{/owner}{/repo}",
          "subscriptions_url": "https://api.github.com/users/mumoshu/subscriptions",
          "organizations_url": "https://api.github.com/users/mumoshu/orgs",
          "repos_url": "https://api.github.com/users/mumoshu/repos",
          "events_url": "https://api.github.com/users/mumoshu/events{/privacy}",
          "received_events_url": "https://api.github.com/users/mumoshu/received_events",
          "type": "User",
          "site_admin": false
        },
        "content_type": "text/plain; charset=utf-8",
        "state": "uploaded",
        "size": 398,
        "download_count": 0,
        "created_at": "2019-06-25T10:40:43Z",
        "updated_at": "2019-06-25T10:40:43Z",
        "browser_download_url": "https://github.com/mumoshu/variant/releases/download/v0.31.1/variant_0.31.1_checksums.txt"
      },
      {
        "url": "https://api.github.com/repos/mumoshu/variant/releases/assets/13388584",
        "id": 13388584,
        "node_id": "MDEyOlJlbGVhc2VBc3NldDEzMzg4NTg0",
        "name": "variant_0.31.1_darwin_386.tar.gz",
        "label": "",
        "uploader": {
          "login": "mumoshu",
          "id": 22009,
          "node_id": "MDQ6VXNlcjIyMDA5",
          "avatar_url": "https://avatars0.githubusercontent.com/u/22009?v=4",
          "gravatar_id": "",
          "url": "https://api.github.com/users/mumoshu",
          "html_url": "https://github.com/mumoshu",
          "followers_url": "https://api.github.com/users/mumoshu/followers",
          "following_url": "https://api.github.com/users/mumoshu/following{/other_user}",
          "gists_url": "https://api.github.com/users/mumoshu/gists{/gist_id}",
          "starred_url": "https://api.github.com/users/mumoshu/starred{/owner}{/repo}",
          "subscriptions_url": "https://api.github.com/users/mumoshu/subscriptions",
          "organizations_url": "https://api.github.com/users/mumoshu/orgs",
          "repos_url": "https://api.github.com/users/mumoshu/repos",
          "events_url": "https://api.github.com/users/mumoshu/events{/privacy}",
          "received_events_url": "https://api.github.com/users/mumoshu/received_events",
          "type": "User",
          "site_admin": false
        },
        "content_type": "application/gzip",
        "state": "uploaded",
        "size": 5760736,
        "download_count": 1,
        "created_at": "2019-06-25T10:40:43Z",
        "updated_at": "2019-06-25T10:40:43Z",
        "browser_download_url": "https://github.com/mumoshu/variant/releases/download/v0.31.1/variant_0.31.1_darwin_386.tar.gz"
      },
      {
        "url": "https://api.github.com/repos/mumoshu/variant/releases/assets/13388588",
        "id": 13388588,
        "node_id": "MDEyOlJlbGVhc2VBc3NldDEzMzg4NTg4",
        "name": "variant_0.31.1_darwin_amd64.tar.gz",
        "label": "",
        "uploader": {
          "login": "mumoshu",
          "id": 22009,
          "node_id": "MDQ6VXNlcjIyMDA5",
          "avatar_url": "https://avatars0.githubusercontent.com/u/22009?v=4",
          "gravatar_id": "",
          "url": "https://api.github.com/users/mumoshu",
          "html_url": "https://github.com/mumoshu",
          "followers_url": "https://api.github.com/users/mumoshu/followers",
          "following_url": "https://api.github.com/users/mumoshu/following{/other_user}",
          "gists_url": "https://api.github.com/users/mumoshu/gists{/gist_id}",
          "starred_url": "https://api.github.com/users/mumoshu/starred{/owner}{/repo}",
          "subscriptions_url": "https://api.github.com/users/mumoshu/subscriptions",
          "organizations_url": "https://api.github.com/users/mumoshu/orgs",
          "repos_url": "https://api.github.com/users/mumoshu/repos",
          "events_url": "https://api.github.com/users/mumoshu/events{/privacy}",
          "received_events_url": "https://api.github.com/users/mumoshu/received_events",
          "type": "User",
          "site_admin": false
        },
        "content_type": "application/gzip",
        "state": "uploaded",
        "size": 6095610,
        "download_count": 6,
        "created_at": "2019-06-25T10:40:43Z",
        "updated_at": "2019-06-25T10:40:44Z",
        "browser_download_url": "https://github.com/mumoshu/variant/releases/download/v0.31.1/variant_0.31.1_darwin_amd64.tar.gz"
      },
      {
        "url": "https://api.github.com/repos/mumoshu/variant/releases/assets/13388585",
        "id": 13388585,
        "node_id": "MDEyOlJlbGVhc2VBc3NldDEzMzg4NTg1",
        "name": "variant_0.31.1_linux_386.tar.gz",
        "label": "",
        "uploader": {
          "login": "mumoshu",
          "id": 22009,
          "node_id": "MDQ6VXNlcjIyMDA5",
          "avatar_url": "https://avatars0.githubusercontent.com/u/22009?v=4",
          "gravatar_id": "",
          "url": "https://api.github.com/users/mumoshu",
          "html_url": "https://github.com/mumoshu",
          "followers_url": "https://api.github.com/users/mumoshu/followers",
          "following_url": "https://api.github.com/users/mumoshu/following{/other_user}",
          "gists_url": "https://api.github.com/users/mumoshu/gists{/gist_id}",
          "starred_url": "https://api.github.com/users/mumoshu/starred{/owner}{/repo}",
          "subscriptions_url": "https://api.github.com/users/mumoshu/subscriptions",
          "organizations_url": "https://api.github.com/users/mumoshu/orgs",
          "repos_url": "https://api.github.com/users/mumoshu/repos",
          "events_url": "https://api.github.com/users/mumoshu/events{/privacy}",
          "received_events_url": "https://api.github.com/users/mumoshu/received_events",
          "type": "User",
          "site_admin": false
        },
        "content_type": "application/gzip",
        "state": "uploaded",
        "size": 5495526,
        "download_count": 0,
        "created_at": "2019-06-25T10:40:43Z",
        "updated_at": "2019-06-25T10:40:43Z",
        "browser_download_url": "https://github.com/mumoshu/variant/releases/download/v0.31.1/variant_0.31.1_linux_386.tar.gz"
      },
      {
        "url": "https://api.github.com/repos/mumoshu/variant/releases/assets/13388586",
        "id": 13388586,
        "node_id": "MDEyOlJlbGVhc2VBc3NldDEzMzg4NTg2",
        "name": "variant_0.31.1_linux_amd64.tar.gz",
        "label": "",
        "uploader": {
          "login": "mumoshu",
          "id": 22009,
          "node_id": "MDQ6VXNlcjIyMDA5",
          "avatar_url": "https://avatars0.githubusercontent.com/u/22009?v=4",
          "gravatar_id": "",
          "url": "https://api.github.com/users/mumoshu",
          "html_url": "https://github.com/mumoshu",
          "followers_url": "https://api.github.com/users/mumoshu/followers",
          "following_url": "https://api.github.com/users/mumoshu/following{/other_user}",
          "gists_url": "https://api.github.com/users/mumoshu/gists{/gist_id}",
          "starred_url": "https://api.github.com/users/mumoshu/starred{/owner}{/repo}",
          "subscriptions_url": "https://api.github.com/users/mumoshu/subscriptions",
          "organizations_url": "https://api.github.com/users/mumoshu/orgs",
          "repos_url": "https://api.github.com/users/mumoshu/repos",
          "events_url": "https://api.github.com/users/mumoshu/events{/privacy}",
          "received_events_url": "https://api.github.com/users/mumoshu/received_events",
          "type": "User",
          "site_admin": false
        },
        "content_type": "application/gzip",
        "state": "uploaded",
        "size": 5822584,
        "download_count": 37,
        "created_at": "2019-06-25T10:40:43Z",
        "updated_at": "2019-06-25T10:40:43Z",
        "browser_download_url": "https://github.com/mumoshu/variant/releases/download/v0.31.1/variant_0.31.1_linux_amd64.tar.gz"
      }
    ],
    "tarball_url": "https://api.github.com/repos/mumoshu/variant/tarball/v0.31.1",
    "zipball_url": "https://api.github.com/repos/mumoshu/variant/zipball/v0.31.1",
    "body": "## Changelog\n\nc78b3b7 Improvement #97: Inverse inherited params processing in tasks (#96)\n3080240 Remove unused viper variable (#98)\n\n"
  }
]
`

	gets := map[vhttpget.TestGetInput]string{
		vhttpget.TestGetInput{URL: "https://api.github.com/repos/mumoshu/variant/releases"}: expectedOut,
	}
	httpGetter := vhttpget.NewTester(gets)
	stable, err := New(conf.ReleaseChannel, HttpGetter(httpGetter))
	if err != nil {
		t.Fatal(err)
	}

	latest, err := stable.Latest("= 0.31.1")
	if err != nil {
		t.Fatal(err)
	}

	expected := "0.31.1"
	if latest.Version != expected {
		t.Errorf("unexpected version: expected=%v, got=%v", expected, latest.Version)
	}
}

func TestProvider_DockerRegistryImageTags(t *testing.T) {
	input := `releaseChannel:
  versionsFrom:
    # This basically fetch "curl https://registry.hub.docker.com/v2/repositories/mumoshu/helmfile-chatops/tags/ | jq -r .results[].name"
    dockerImageTags:
      source: mumoshu/helmfile-chatops
`

	conf := &Config{}
	if err := yaml.Unmarshal([]byte(input), conf); err != nil {
		t.Fatal(err)
	}

	gets := map[vhttpget.TestGetInput]string{
		vhttpget.TestGetInput{URL: "https://registry.hub.docker.com/v2/repositories/mumoshu/helmfile-chatops/tags/?page_size=1000"}: `{"count": 2, "next": null, "previous": null, "results": [{"name": "0.2.0", "full_size": 89867735, "images": [{"size": 89867735, "architecture": "amd64", "variant": null, "features": null, "os": "linux", "os_version": null, "os_features": null}], "id": 60688451, "repository": 7345782, "creator": 17205, "last_updater": 17205, "last_updated": "2019-07-02T07:02:05.424914Z", "image_id": null, "v2": true}, {"name": "0.1.0", "full_size": 89738457, "images": [{"size": 89738457, "architecture": "amd64", "variant": null, "features": null, "os": "linux", "os_version": null, "os_features": null}], "id": 60687743, "repository": 7345782, "creator": 17205, "last_updater": 17205, "last_updated": "2019-07-02T06:51:44.860914Z", "image_id": null, "v2": true}]}`,
	}
	httpGetter := vhttpget.NewTester(gets)
	stable, err := New(conf.ReleaseChannel, HttpGetter(httpGetter))
	if err != nil {
		t.Fatal(err)
	}

	latest, err := stable.Latest("= 0.2.0")
	if err != nil {
		t.Fatal(err)
	}

	expected := "0.2.0"
	if latest.Version != expected {
		t.Errorf("unexpected version: expected=%v, got=%v", expected, latest.Version)
	}
}
