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
			return mod.Run()
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

	modlistver := &cobra.Command{
		Use: "list-versions",
		RunE: func(cmd *cobra.Command, args []string) error {
			man, err := variantmod.New(variantmod.Logger(log))
			if err != nil {
				return err
			}
			mod, err := man.Load()
			if err != nil {
				return err
			}
			return mod.ListVersions(os.Stdout)
		},
	}

	modlistdepver := &cobra.Command{
		Use: "list-dependency-versions",
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
			return mod.ListDependencyVersions(args[0], os.Stdout)
		},
	}

	cmd.AddCommand(modexec)
	cmd.AddCommand(modlistver)
	cmd.AddCommand(modlistdepver)

	cmd.SilenceErrors = true

	fs := loginfra.Init()

	// Hand parsing of remaining flags to pflags and cobra
	pflag.CommandLine.AddGoFlagSet(fs)

	if err := cmd.Execute(); err != nil {
		log.Error(err, err.Error())
		os.Exit(1)
	}
}
