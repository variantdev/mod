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

	cmd.SilenceErrors = true

	fs := loginfra.Init()

	// Hand parsing of remaining flags to pflags and cobra
	pflag.CommandLine.AddGoFlagSet(fs)

	if err := cmd.Execute(); err != nil {
		log.Error(err, err.Error())
		os.Exit(1)
	}
}
