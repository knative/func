package v06

import "github.com/buildpacks/lifecycle/cmd"

var exitCodes = map[cmd.LifecycleExitError]int{
	// detect phase errors: 20-29
	cmd.FailedDetect:           20, // FailedDetect indicates that no buildpacks detected
	cmd.FailedDetectWithErrors: 21, // FailedDetectWithErrors indicated that no buildpacks detected and at least one errored
	cmd.DetectError:            22, // DetectError indicates generic detect error

	// analyze phase errors: 30-39
	cmd.AnalyzeError: 32, // AnalyzeError indicates generic analyze error

	// restore phase errors: 40-49
	cmd.RestoreError: 42, // RestoreError indicates generic restore error

	// build phase errors: 50-59
	cmd.FailedBuildWithErrors: 51, // FailedBuildWithErrors indicates buildpack error during /bin/build
	cmd.BuildError:            52, // BuildError indicates generic build error

	// export phase errors: 60-69
	cmd.ExportError: 62, // ExportError indicates generic export error

	// rebase phase errors: 70-79
	cmd.RebaseError: 72, // RebaseError indicates generic rebase error

	// launch phase errors: 80-89
	cmd.LaunchError: 82, // LaunchError indicates generic launch error
}

func (p *Platform) CodeFor(errType cmd.LifecycleExitError) int {
	if code, ok := exitCodes[errType]; ok {
		return code
	}
	return cmd.CodeFailed
}
