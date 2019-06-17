package execversionmanager

import (
	"k8s.io/klog"
	"os"
	"runtime"
)

func getMatchingPlatform(p Executable) (Platform, bool, error) {
	os, arch := osArch()
	klog.V(1).Infof("Using os=%s arch=%s", os, arch)
	return matchPlatformToSystemEnvs(p, os, arch)
}

// osArch returns the OS/arch combination to be used on the current system. It
// can be overridden by setting VARIANT_MOD_OS and/or VARIANT_MOD_ARCH environment variables.
func osArch() (string, string) {
	goos, goarch := runtime.GOOS, runtime.GOARCH
	envOS, envArch := os.Getenv("VARIANT_MOD_OS"), os.Getenv("VARIANT_MOD_ARCH")
	if envOS != "" {
		goos = envOS
	}
	if envArch != "" {
		goarch = envArch
	}
	return goos, goarch
}

func matchPlatformToSystemEnvs(p Executable, os, arch string) (Platform, bool, error) {
	ls := map[string]string{
		"os":   os,
		"arch": arch,
	}
	klog.V(2).Infof("Matching platform for labels(%v)", ls)
	for i, platform := range p.Platforms {
		if platform.Selector.Matches(ls) {
			klog.V(1).Infof("Found matching platform with index (%d)", i)
			return platform, true, nil
		}
	}
	return Platform{}, false, nil
}
