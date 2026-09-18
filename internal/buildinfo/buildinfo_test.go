package buildinfo

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetFallsBackToDev(t *testing.T) {
	info := Get()
	assert.NotEmpty(t, info.Version, "an unstamped build still reports a version")
	assert.Equal(t, runtime.Version(), info.GoVersion)
	assert.Equal(t, runtime.GOOS+"/"+runtime.GOARCH, info.Platform)
}

func TestGetPrefersStampedValues(t *testing.T) {
	t.Cleanup(func(prevVersion, prevCommit, prevDate string) func() {
		return func() { version, commit, date = prevVersion, prevCommit, prevDate }
	}(version, commit, date))

	version, commit, date = "v1.2.3", "abc1234", "2026-01-02T03:04:05Z"

	info := Get()
	assert.Equal(t, "v1.2.3", info.Version)
	assert.Equal(t, "abc1234", info.Commit)
	assert.Equal(t, "2026-01-02T03:04:05Z", info.Date)
	assert.Equal(t, "v1.2.3", info.Short())
}

func TestStringIncludesEveryKnownField(t *testing.T) {
	info := Info{
		Version:   "v1.2.3",
		Commit:    "abc1234",
		Date:      "2026-01-02T03:04:05Z",
		GoVersion: "go1.27.0",
		Platform:  "linux/amd64",
	}

	out := info.String()
	for _, want := range []string{"v1.2.3", "abc1234", "2026-01-02T03:04:05Z", "go1.27.0", "linux/amd64"} {
		assert.Contains(t, out, want)
	}
}

func TestStringOmitsUnknownFields(t *testing.T) {
	out := Info{Version: "dev", GoVersion: "go1.27.0", Platform: "linux/amd64"}.String()
	assert.NotContains(t, out, "commit:")
	assert.NotContains(t, out, "built:")
	assert.Contains(t, out, "version:    dev")
}

func TestStampedCommitIsNotMarkedDirty(t *testing.T) {
	t.Cleanup(func(prev string) func() { return func() { commit = prev } }(commit))

	commit = "abc1234"
	assert.Equal(t, "abc1234", Get().Commit,
		"a commit passed in via -ldflags must be reported verbatim")
}
