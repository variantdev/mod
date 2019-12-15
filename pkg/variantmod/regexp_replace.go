package variantmod

import (
	"regexp"
)

func regexpReplace(source []byte, pat *regexp.Regexp, template string) ([]byte, error) {
	var (
		cur int
		res []byte
	)
	for _, m := range pat.FindAllSubmatchIndex(source, -1) {
		res = append(res, source[cur:m[0]]...)
		res = pat.Expand(res, []byte(template), source, m)
		cur = m[1]
	}
	res = append(res, source[cur:]...)
	return res, nil
}
