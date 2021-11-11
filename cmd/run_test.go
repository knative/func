package cmd

import (
	"fmt"
	"os"
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

	tempDir, err := os.MkdirTemp("", "func-tests")
	if err != nil {
		t.Fatalf("temp dir couldn't be created %v", err)
	}
	defer os.RemoveAll(tempDir)

	fullPath := tempDir + "/func.yaml"
	tempFile, err := os.Create(fullPath)
	if err != nil {
		t.Fatalf("temp file couldn't be created %v", err)
	}

	fmt.Println(tempDir)
	viper.SetDefault("path", tempDir)
	fmt.Println("VIPER", viper.GetString("path"))

	cmd := cobra.Command{}
	// Assigns context to context.Background
	_, err = cmd.ExecuteC()

	if err != nil {
		t.Fatalf("failed to assign cmd context %v", err)
	}

	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "Non built func won't execute if build is skipped",
			args: args{
				cmd:      &cmd,
				args:     nil,
				clientFn: newRunClient,
				fileContents: `name: test-func
runtime: go`,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		_, err := tempFile.WriteString(tt.args.fileContents)
		if err != nil {
			t.Fatalf("file content was not written %v", err)
		}
		t.Run(tt.name, func(t *testing.T) {
			if err := runRun(tt.args.cmd, tt.args.args, tt.args.clientFn); (err != nil) != tt.wantErr {
				t.Errorf("runRun() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
