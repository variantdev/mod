package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/variantdev/mod/pkg/loginfra"
	"github.com/variantdev/mod/pkg/variantmod"
	"k8s.io/klog/klogr"
	"os"
	"time"
)

func Execute() {
	log := klogr.New()

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

	up := func(branch, title, base string, build, push, pr bool) error {
		man, err := variantmod.New(variantmod.Logger(log))
		if err != nil {
			return err
		}
		if err := man.Up(); err != nil {
			return err
		}
		files := []string{"variant.mod"}
		if build {
			r, err := man.Build()
			if err != nil {
				return err
			}
			files = r.Files
		}
		if pr {
			push = true
		}
		ts := time.Now().Format("20060102150405")
		branch = fmt.Sprintf("%s-%s", branch, ts)
		if push {
			if err := man.Push(files, branch); err != nil {
				return err
			}
		}
		if pr {
			if err := man.PullRequest(title, base, branch); err != nil {
				return err
			}
		}
		return nil
	}

	{
		var repo, branch, base, title string
		var build, push, pr bool
		modup := &cobra.Command{
			Use: "up",
			RunE: func(cmd *cobra.Command, args []string) error {
				return up(branch, title, base, build, push, pr)
			},
		}
		modup.Flags().BoolVar(&build, "build", false, "Run `build` after update")
		modup.Flags().BoolVar(&push, "push", false, "Push to Git repository after update (and `build` if --build provided)")
		modup.Flags().StringVar(&repo, "repo", "", "Git repository to which the provisioned files are pushed")
		modup.Flags().StringVar(&branch, "branch", "mod-up", "Prefix of git branch name to which the provisioned files are pushed")
		modup.Flags().StringVar(&base, "base", "master", "Branch to which pull request is sent to")
		modup.Flags().BoolVar(&pr, "pull-request", false, "Send a pull request after push. Implies --push")
		modup.Flags().StringVar(&title, "title", "Update dependencies", "Title of the pull-request to be sent")
		cmd.AddCommand(modup)
	}

	{
		var branch, base, title string
		var build, push, pr, public bool
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

				return up(branch, title, base, build, push, pr)
			},
		}
		modcreate.Flags().BoolVar(&build, "build", true, "Run `build` after update")
		modcreate.Flags().BoolVar(&push, "push", true, "Push to Git repository after update (and `build` if --build provided)")
		modcreate.Flags().StringVar(&branch, "branch", "mod-init", "Prefix of git branch name to which the provisioned files are pushed")
		modcreate.Flags().StringVar(&base, "base", "master", "Branch to which pull request is sent to")
		modcreate.Flags().BoolVar(&pr, "pull-request", false, "Send a pull request after push. Implies --push")
		modcreate.Flags().BoolVar(&public, "public", false, "Make the repository public")
		modcreate.Flags().StringVar(&title, "title", "Initialize repository", "Title of the pull-request to be sent")
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

	if err := cmd.Execute(); err != nil {
		log.Error(err, err.Error())
		os.Exit(1)
	}
}
