package cogito

import (
	"errors"
	"os"
	"testing"

	"gotest.tools/v3/assert"
)

func TestValidateInputDirFailure(t *testing.T) {
	err := validateInputDir("testdata/two-dirs", "dummy-owner", "dummy-repo")

	assert.Error(t, err, "found 2 input dirs: [dir-1 dir-2]. Want exactly 1, corresponding to the GitHub repo dummy-owner/dummy-repo")
}

func TestCollectInputDirs(t *testing.T) {
	var testCases = []struct {
		name    string
		dir     string
		wantErr error
		wantN   int
	}{
		{
			name:    "non existing base directory",
			dir:     "non-existing",
			wantErr: os.ErrNotExist,
			wantN:   0,
		},
		{
			name:    "empty directory",
			dir:     "testdata/empty-dir",
			wantErr: nil,
			wantN:   0,
		},
		{
			name:    "two directories and one file",
			dir:     "testdata/two-dirs",
			wantErr: nil,
			wantN:   2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dirs, err := collectInputDirs(tc.dir)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("sut(%v): error: have %v; want %v", tc.dir, err, tc.wantErr)
			}
			gotN := len(dirs)
			if gotN != tc.wantN {
				t.Errorf("sut(%v): len(dirs): have %v; want %v", tc.dir, gotN, tc.wantN)
			}
		})
	}
}
