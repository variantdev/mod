package loginfra

import (
	"flag"
	"fmt"
	"io/ioutil"
	"k8s.io/klog"
	"os"
	"strings"
)

func NewFlagSet() *flag.FlagSet {
	// See https://flowerinthenight.com/blog/2019/02/05/golang-cobra-klog
	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	// Suppress usage flag.ErrHelp
	fs.SetOutput(ioutil.Discard)

	return fs
}

func Init() *flag.FlagSet {
	fs := NewFlagSet()

	fs = AddKlogFlags(fs)

	return Parse(fs)
}

func Parse(fs *flag.FlagSet) *flag.FlagSet {
	args := append([]string{}, os.Args[1:]...)

	if err := fs.Parse(args); err != nil && err != flag.ErrHelp && !strings.Contains(err.Error(), "flag provided but not defined") {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	//remainings := fs.Args()

	return fs
}

func AddKlogFlags(fs *flag.FlagSet) *flag.FlagSet {
	klog.InitFlags(fs)

	// Configure klog
	fs.Set("skip_headers", "true")

	v := os.Getenv("VARIANT_MOD_VERBOSITY")
	if v != "" {
		// -v LEVEL must preceed the remaining args to be parsed by fs
		fmt.Fprintf(os.Stderr, "Setting log verbosity to %s\n", v)
		fs.Set("v", v)
	}

	return fs
}
