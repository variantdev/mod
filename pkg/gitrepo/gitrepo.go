package gitrepo

import (
	"context"
	"fmt"
	"github.com/google/go-github/v27/github"
	"golang.org/x/oauth2"
	"os"
	"strconv"
	"strings"
)

type Client struct {
	github *github.Client
}

type NewRepositoryOption struct {
	Private bool
	TemplateOwner string
	TemplateRepo string
}

func (c *Client) NewRepository(ctx context.Context, owner string, repo string, opt *NewRepositoryOption) (*github.Repository, error) {
	req := github.TemplateRepoRequest{
		Name:    &owner,
		Owner:   &repo,
		Private: &opt.Private,
	}
	createdRepo, _, err := c.github.Repositories.CreateFromTemplate(ctx, opt.TemplateOwner, opt.TemplateRepo, &req)
	return createdRepo, err
}

type Query struct {
	State string
	Body string
	Title string
}

func (q *Query) Filtter(issues []github.Issue) []*github.Issue {
	result := make([]*github.Issue, 0)
	for _, issue := range issues {
		if issue.GetState() == q.State {
			result = append(result, &issue)
			continue
		}
		if issue.GetBody() == q.Body {
			result = append(result, &issue)
			continue
		}
		if issue.GetTitle() == q.Title {
			result = append(result, &issue)
			continue
		}
	}
	return result
}

func (q *Query) String() string {
	arr := make([]string, 0)
	if q.State != "" {
		arr = append(arr, fmt.Sprintf("is:%s", q.State))
	}
	if q.Body != "" {
		arr = append(arr, fmt.Sprintf("%s in:body", strconv.Quote(strings.Replace(q.Body, "\n", " ", -1))))
	}
	if q.Title != "" {
		arr = append(arr, fmt.Sprintf("%s in:title", strconv.Quote(q.Title)))
	}
	return strings.Join(arr, " ")
}

func (c *Client) SearchIssues(ctx context.Context, owner string, repo string, query *Query) ([]*github.Issue, error) {
	q := fmt.Sprintf("is:pr repo:%s/%s %s", owner, repo, query.String())
	searchOpt := &github.SearchOptions{
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}
	result, resp, err := c.github.Search.Issues(ctx, q, searchOpt)
	if err != nil {
		return nil, err
	}
	issues := query.Filtter(result.Issues)
	for resp.NextPage > searchOpt.Page {
		searchOpt.Page = resp.NextPage
		r, res, err := c.github.Search.Issues(ctx, q, searchOpt)
		if err != nil {
			return nil, err
		}
		issues = append(issues, query.Filtter(r.Issues)...)
		resp = res
	}
	return issues, nil
}

type NewPullRequestOptions struct {
	Title string
	Head string
	Base string
	Body string
}

func (c *Client) NewPullRequest(ctx context.Context, owner string, repo string, opt *NewPullRequestOptions) (*github.PullRequest, error) {
	newPr := github.NewPullRequest{
		Title: &opt.Title,
		Head:  &opt.Head,
		Base:  &opt.Base,
		Body:  &opt.Body,
	}
	pr, _, err := c.github.PullRequests.Create(ctx, owner, repo, &newPr)

	return pr, err
}

func NewClient(ctx context.Context) *Client {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
	)
	tc := oauth2.NewClient(ctx, ts)
	gc := github.NewClient(tc)

	return &Client{
		github: gc,
	}
}