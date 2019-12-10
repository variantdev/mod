package cmd

import (
	"fmt"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/variantdev/mod/pkg/cmdsite"
	"github.com/variantdev/mod/pkg/loginfra"
	"github.com/variantdev/mod/pkg/variantmod"
	"k8s.io/klog/klogr"
	"os"
	"strings"
	"time"
)

func New(log logr.Logger) *cobra.Command {
	cmd := cobra.Command{
		Use: "mod",
		RunE: func(cmd *cobra.Command, args []string) error {
			mod, err := variantmod.New(variantmod.Logger(log))
			if err != nil {
				return err
			}
			_, err = mod.Build()
			return err
		},
	}

	modbuild := &cobra.Command{
		Use: "build",
		RunE: func(cmd *cobra.Command, args []string) error {
			mod, err := variantmod.New(variantmod.Logger(log))
			if err != nil {
				return err
			}
			_, err = mod.Build()
			return err
		},
	}

	modexec := &cobra.Command{
		Use: "exec",
		RunE: func(cmd *cobra.Command, args []string) error {
			man, err := variantmod.New(variantmod.Logger(log))
			if err != nil {
				return err
			}
			mod, err := man.Load()
			if err != nil {
				return err
			}
			sh, err := mod.Shell()
			if err != nil {
				return err
			}
			return sh.RunCommand(args[0], args[1:], os.Stdout, os.Stderr)
		},
	}

	modlistdepver := &cobra.Command{
		Use:  "list-dependency-versions",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			man, err := variantmod.New(variantmod.Logger(log))
			if err != nil {
				return err
			}
			mod, err := man.Load()
			if err != nil {
				return err
			}
			return mod.ListVersions(args[0], os.Stdout)
		},
	}

	up := func(branch, title, body, base string, build, push, pr, skipDuplicatePRBody, skipDuplicatePRTitle bool) error {
		if pr {
			push = true
		}
		man, err := variantmod.New(
			variantmod.Logger(log),
			variantmod.Commander(cmdsite.DefaultRunCommand),
		)
		if err != nil {
			return err
		}
		if err := man.Checkout(base); err != nil {
			return err
		}
		if err := man.Up(); err != nil {
			return err
		}
		files := []string{"variant.mod", "variant.lock"}
		if !strings.HasPrefix(base, branch) && push {
			ts := time.Now().Format("20060102150405")
			branch = fmt.Sprintf("%s-%s", branch, ts)
			if err := man.Checkout(branch); err != nil {
				return err
			}
		} else {
			branch = base
		}
		if build {
			r, err := man.Build()
			if err != nil {
				return err
			}
			files = append(files, r.Files...)
		}
		var pushed bool
		if push {
			pushed, err = man.Push(files, branch)
			if err != nil {
				return err
			}
			if base == branch {
				return nil
			}
		}
		if pr && pushed {
			if err := man.PullRequest(title, body, base, branch, skipDuplicatePRBody, skipDuplicatePRTitle); err != nil {
				return err
			}
		}
		return nil
	}

	{
		var repo, branch, base, title, body string
		var build, push, pr, skipDuplicatePRBody, skipDuplicatePRTitle bool
		modup := &cobra.Command{
			Use: "up",
			RunE: func(cmd *cobra.Command, args []string) error {
				return up(branch, title, body, base, build, push, pr, skipDuplicatePRBody, skipDuplicatePRTitle)
			},
		}
		modup.Flags().BoolVar(&build, "build", false, "Run `build` after update")
		modup.Flags().BoolVar(&push, "push", false, "Push to Git repository after update (and `build` if --build provided)")
		modup.Flags().StringVar(&repo, "repo", "", "Git repository to which the provisioned files are pushed")
		modup.Flags().StringVar(&branch, "branch", "mod-up", "Prefix of git branch name to which the provisioned files are pushed")
		modup.Flags().StringVar(&base, "base", "master", "Branch to which pull request is sent to")
		modup.Flags().BoolVar(&pr, "pull-request", false, "Send a pull request after push. Implies --push")
		modup.Flags().StringVar(&title, "title", "Update dependencies", "Title of the pull-request to be sent")
		modup.Flags().StringVar(&body, "body", "{{ .RawLock | sha256 }}", "Title of the pull-request to be sent")
		modup.Flags().BoolVar(&skipDuplicatePRBody, "skip-duplicate-pull-request-body", false, "If true, PR creation will be skipped if the PR body is duplicated.")
		modup.Flags().BoolVar(&skipDuplicatePRTitle, "skip-duplicate-pull-request-title", false, "If true, PR creation will be skipped if the PR title is duplicated.")
		cmd.AddCommand(modup)
	}

	{
		var branch, base, title, body string
		var build, push, pr, public, skipDuplicatePRBody, skipDuplicatePRTitle bool
		modcreate := &cobra.Command{
			Use:  "create TEMPLATE_REPO NEW_REPO",
			Args: cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				templateRepo := args[0]
				newRepo := args[1]
				man, err := variantmod.New(variantmod.Logger(log))
				if err != nil {
					return err
				}

				if err := man.Create(templateRepo, newRepo, public); err != nil {
					return err
				}

				return up(branch, title, body, base, build, push, pr, skipDuplicatePRBody, skipDuplicatePRTitle)
			},
		}
		modcreate.Flags().BoolVar(&build, "build", true, "Run `build` after update")
		modcreate.Flags().BoolVar(&push, "push", true, "Push to Git repository after update (and `build` if --build provided)")
		modcreate.Flags().StringVar(&branch, "branch", "mod-init", "Prefix of git branch name to which the provisioned files are pushed")
		modcreate.Flags().StringVar(&base, "base", "master", "Branch to which pull request is sent to")
		modcreate.Flags().BoolVar(&pr, "pull-request", false, "Send a pull request after push. Implies --push")
		modcreate.Flags().BoolVar(&public, "public", false, "Make the repository public")
		modcreate.Flags().StringVar(&title, "title", "Initialize repository", "Title of the pull-request to be sent")
		modcreate.Flags().StringVar(&body, "body", "{{ .RawLock | sha256 }}", "Title of the pull-request to be sent")
		modcreate.Flags().BoolVar(&skipDuplicatePRBody, "skip-duplicate-pull-request-body", false, "If true, PR creation will be skipped if the PR body is duplicated.")
		modcreate.Flags().BoolVar(&skipDuplicatePRTitle, "skip-duplicate-pull-request-title", false, "If true, PR creation will be skipped if the PR title is duplicated.")
		cmd.AddCommand(modcreate)
	}

	modprovision := &cobra.Command{
		Use: "provision",
		RunE: func(cmd *cobra.Command, args []string) error {
			man, err := variantmod.New(variantmod.Logger(log))
			if err != nil {
				return err
			}
			_, err = man.Build()
			return err
		},
	}

	cmd.AddCommand(modbuild)
	cmd.AddCommand(modexec)
	cmd.AddCommand(modlistdepver)
	cmd.AddCommand(modprovision)

	cmd.SilenceErrors = true

	fs := loginfra.Init()

	// Hand parsing of remaining flags to pflags and cobra
	pflag.CommandLine.AddGoFlagSet(fs)

	return &cmd
}

func Execute() {
	log := klogr.New()

	cmd := New(log)

	if err := cmd.Execute(); err != nil {
		log.Error(err, err.Error())
		os.Exit(1)
	}
}
