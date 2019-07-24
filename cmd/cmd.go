package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/variantdev/mod/pkg/loginfra"
	"github.com/variantdev/mod/pkg/variantmod"
	"k8s.io/klog/klogr"
	"os"
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

	var repo, branch string
	var build bool
	var push bool
	modup := &cobra.Command{
		Use: "up",
		RunE: func(cmd *cobra.Command, args []string) error {
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
			if push {
				if err := man.Push(files, repo, branch); err != nil {
					return err
				}
			}
			return nil
		},
	}
	modup.Flags().BoolVar(&build, "build", false, "Run `build` after update")
	modup.Flags().BoolVar(&push, "push", false, "Push to Git repository after update (and `build` if --build provided)")
	modup.Flags().StringVar(&repo, "repo", "", "Git repository to which the provisioned files are pushed")
	modup.Flags().StringVar(&branch, "branch", "master", "Git branch to which the provisioned files are pushed")

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
	cmd.AddCommand(modup)
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
