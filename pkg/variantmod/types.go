package variantmod

import (
	"io"

	"github.com/variantdev/mod/pkg/cmdsite"
)

type Interface interface {
	Create(templateRepo, newRepo string, public bool) error
	Build() (*BuildResult, error)
	Up() error
	Shell() (*cmdsite.CommandSite, error)
	ListVersions(depName string, out io.Writer) error
	Checkout(branch string) error
	Push(files []string, branch string) (bool, error)
	PullRequest(title, body, base, head string, skipDuplicatePRBody, skipDuplicatePRTitle bool) error
}

