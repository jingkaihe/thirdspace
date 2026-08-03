package version

import "fmt"

var (
	// Version is replaced at build time from VERSION.txt.
	Version = "dev"
	// GitCommit is replaced at build time with the source commit SHA.
	GitCommit = "unknown"
)

func String() string {
	return fmt.Sprintf("%s (commit %s)", Version, GitCommit)
}
