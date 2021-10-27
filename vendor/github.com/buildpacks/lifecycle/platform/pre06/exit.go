package pre06

import (
	"github.com/buildpacks/lifecycle/cmd"
)

var exitCodes = map[cmd.LifecycleExitError]int{
	// detect phase errors: 100-199
	cmd.FailedDetect:           100, // FailedDetect indicates that no buildpacks detected
	cmd.FailedDetectWithErrors: 101, // FailedDetectWithErrors indicated that no buildpacks detected and at least one errored
	cmd.DetectError:            102, // DetectError indicates generic detect error

	// analyze phase errors: 200-299
	cmd.AnalyzeError: 202, // AnalyzeError indicates generic analyze error

	// restore phase errors: 300-399
	cmd.RestoreError: 302, // RestoreError indicates generic restore error

	// build phase errors: 400-499
	cmd.FailedBuildWithErrors: 401, // FailedBuildWithErrors indicates buildpack error during /bin/build
	cmd.BuildError:            402, // BuildError indicates generic build error

	// export phase errors: 500-599
	cmd.ExportError: 502, // ExportError indicates generic export error

	// rebase phase errors: 600-699
	cmd.RebaseError: 602, // RebaseError indicates generic rebase error

	// launch phase errors: 700-799
	cmd.LaunchError: 702, // LaunchError indicates generic launch error
}

func (p *pre06Platform) CodeFor(errType cmd.LifecycleExitError) int {
	if code, ok := exitCodes[errType]; ok {
		return code
	}
	return cmd.CodeFailed
}
