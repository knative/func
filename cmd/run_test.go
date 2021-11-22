package cmd

import (
	"os"
	"strconv"
	"testing"

	"github.com/ory/viper"
	"github.com/spf13/cobra"
)

func TestRunRun(t *testing.T) {
	type args struct {
		cmd          *cobra.Command
		args         []string
		clientFn     runClientFn
		fileContents string
	}

	cmd := cobra.Command{}
	cmd.Flags().BoolP("build", "b", false, "Build the function only if the function has not been built yet")
	// Assigns context to context.Background
	_, err := cmd.ExecuteC()

	if err != nil {
		t.Fatalf("failed to assign cmd context %v", err)
	}

	tests := []struct {
		name       string
		args       args
		wantErr    bool
		buildFlag  bool
		errMessage string
	}{
		{
			name: "Non built func won't execute if build is skipped",
			args: args{
				cmd:      &cmd,
				args:     nil,
				clientFn: newRunClient,
				fileContents: `name: test-func
runtime: go
created: 2009-11-10 23:00:00`,
			},
			wantErr:    true,
			buildFlag:  false,
			errMessage: "Function has no associated Image. Has it been built?",
		},
		{
			name: "Prebuilt image doesn't build again",
			args: args{
				cmd:      &cmd,
				args:     nil,
				clientFn: newRunClient,
				fileContents: `name: test-func
runtime: go
image: unexistant
created: 2009-11-10 23:00:00`,
			},
			wantErr:    true,
			buildFlag:  true,
			errMessage: "failed to create container: Error response from daemon: No such image: unexistant:latest",
		},
	}
	for _, tt := range tests {
		tempDir, err := os.MkdirTemp("", "func-tests")
		if err != nil {
			t.Fatalf("temp dir couldn't be created %v", err)
		}
		t.Cleanup(func() {
			os.RemoveAll(tempDir)
		})

		fullPath := tempDir + "/func.yaml"
		tempFile, err := os.Create(fullPath)
		if err != nil {
			t.Fatalf("temp file couldn't be created %v", err)
		}

		viper.SetDefault("path", tempDir)

		_, err = tempFile.WriteString(tt.args.fileContents)
		if err != nil {
			t.Fatalf("file content was not written %v", err)
		}
		tt.args.cmd.Flags().Set("build", strconv.FormatBool(tt.buildFlag))
		t.Run(tt.name, func(t *testing.T) {
			err := runRun(tt.args.cmd, tt.args.args, tt.args.clientFn)
			if (err != nil) != tt.wantErr {
				t.Errorf("runRun() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !(err == nil || err.Error() == tt.errMessage) {
				t.Errorf("Expected error message to be %v but got: %v", tt.errMessage, err)
			}
		})
	}
}
