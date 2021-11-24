package cmd

import (
	"os"
	"strconv"
	"testing"
)

func TestRunRun(t *testing.T) {

	tests := []struct {
		name         string
		fileContents string
		wantErr      bool
		buildFlag    bool
		errMessage   string
	}{
		{
			name: "Non built func won't execute if build is skipped",
			fileContents: `name: test-func
runtime: go
created: 2009-11-10 23:00:00`,
			wantErr:    true,
			buildFlag:  false,
			errMessage: "Function has no associated Image. Has it been built? Using the --build flag will build the image it hasn't been built yet",
		},
		{
			name: "Prebuilt image doesn't build again",
			fileContents: `name: test-func
runtime: go
image: unexistant
created: 2009-11-10 23:00:00`,
			wantErr:    true,
			buildFlag:  true,
			errMessage: "failed to create container: Error response from daemon: No such image: unexistant:latest",
		},
	}
	for _, tt := range tests {
		cmd := NewRunCmd(newRunClient)
		tempDir, err := os.MkdirTemp("", "func-tests")
		if err != nil {
			t.Fatalf("temp dir couldn't be created %v", err)
		}
		t.Log("tempDir created:", tempDir)
		t.Cleanup(func() {
			os.RemoveAll(tempDir)
		})

		fullPath := tempDir + "/func.yaml"
		tempFile, err := os.Create(fullPath)
		if err != nil {
			t.Fatalf("temp file couldn't be created %v", err)
		}

		cmd.SetArgs([]string{"--path=" + tempDir, "--build=" + strconv.FormatBool(tt.buildFlag)})

		_, err = tempFile.WriteString(tt.fileContents)
		if err != nil {
			t.Fatalf("file content was not written %v", err)
		}
		if err != nil {
			t.Error("build flag could not be set")
		}
		t.Run(tt.name, func(t *testing.T) {
			err := cmd.Execute()
			if (err != nil) != tt.wantErr {
				t.Errorf("runRun() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !(err == nil || err.Error() == tt.errMessage) {
				t.Errorf("Expected error message to be %v but got: %v", tt.errMessage, err)
			}
		})
	}
}
