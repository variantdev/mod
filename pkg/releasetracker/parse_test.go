package releasetracker

import (
	"fmt"
	"github.com/Masterminds/semver"
	"github.com/google/go-cmp/cmp"
	"k8s.io/klog/klogr"
	"testing"
)

func TestParse(t *testing.T) {
	testcases := []struct {
		ss []string
		rs []*Release
	}{
		{
			[]string{
				"1.2",
				"2.3.4.5",
			},
			[]*Release{
				{Semver: semver.MustParse("1.2"), Version: "1.2"},
				{Semver: semver.MustParse("2.3.4-5"), Version: "2.3.4.5"},
			},
		},
		{
			[]string{
				"1.2.3.4",
				"1.3",
				"1.2",
			},
			[]*Release{
				{Semver: semver.MustParse("1.2"), Version: "1.2"},
				{Semver: semver.MustParse("1.2.3-4"), Version: "1.2.3.4"},
				{Semver: semver.MustParse("1.3"), Version: "1.3"},
			},
		},
	}

	for i := range testcases {
		tc := testcases[i]

		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			tr := &Tracker{
				Logger: klogr.New(),
			}

			rs, err := tr.versionsToReleases(tc.ss)
			if err != nil {
				t.Errorf("%v", err)
			}

			if d := cmp.Diff(tc.rs, rs); d != "" {
				t.Errorf("%s", d)
			}
		})
	}
}
