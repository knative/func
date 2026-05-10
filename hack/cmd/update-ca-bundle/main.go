package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"knative.dev/func/hack/cmd/shared"
)

const (
	caBundlePath = "templates/certs/ca-certificates.crt"
	prTitle      = "chore: update CA bundle"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		cancel()
		<-sigs
		os.Exit(130)
	}()

	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("OK!")
}

func run(ctx context.Context) error {
	owner, repo, err := shared.RepoFromEnv()
	if err != nil {
		return err
	}
	ghClient := shared.NewGHClient(ctx)

	exists, err := shared.PRExists(ctx, ghClient, owner, repo, func(title string) bool {
		return title == prTitle
	})
	if err != nil {
		return fmt.Errorf("cannot check for existing PR: %w", err)
	}
	if exists {
		fmt.Println("The PR already exists!")
		return nil
	}

	if err := shared.RunCmd(ctx, "make", caBundlePath); err != nil {
		return fmt.Errorf("cannot update CA bundle: %w", err)
	}

	changed, err := hasChanges(ctx)
	if err != nil {
		return err
	}
	if !changed {
		fmt.Println("The CA bundle is up to date. Nothing to be done.")
		return nil
	}

	branchName := fmt.Sprintf("update-ca-bundle-%s", time.Now().UTC().Format("2006-01-02"))

	if err := shared.PrepareBranch(ctx, branchName, prTitle, []string{
		"generate/zz_filesystem_generated.go", caBundlePath,
	}); err != nil {
		return fmt.Errorf("cannot prepare branch: %w", err)
	}

	if err := shared.CreatePR(ctx, ghClient, owner, repo, prTitle, fmt.Sprintf("%s:%s", owner, branchName)); err != nil {
		return err
	}
	fmt.Println("The PR has been created!")
	return nil
}

// hasChanges reports whether caBundlePath has uncommitted changes.
// git diff --exit-code exits with 0 when there are no changes, 1 when there are.
func hasChanges(ctx context.Context) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--exit-code", "--", caBundlePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err == nil {
		return false, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return true, nil
	}
	return false, fmt.Errorf("git diff failed unexpectedly: %w", err)
}
