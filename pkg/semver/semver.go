package semver

import (
	"regexp"
	"strings"

	sv "github.com/Masterminds/semver"
)

type Version = sv.Version

var NewConstraint = sv.NewConstraint

func Parse(s string) (*Version, error) {
	fixedS := nonSemverWorkaround(strings.TrimSpace(s))

	return sv.NewVersion(fixedS)
}

var versionRegex *regexp.Regexp

func init() {
	versionRegex = regexp.MustCompile(`v?([0-9]+)(\.[0-9]+)?(\.[0-9]+)?` + `(.*)`)
}

func nonSemverWorkaround(s string) string {
	matches := versionRegex.FindStringSubmatch(s)

	var preLike string

	if len(matches) > 3 {
		preLike = matches[4]
	}

	if preLike != "" && preLike[0] == '.' {
		s = ""
		ss := matches[1:4]
		for i := range ss {
			if ss[i] != "" {
				s += ss[i]
			}
		}

		s += "-" + preLike[1:]
	}

	return s
}
