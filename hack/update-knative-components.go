package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	github "github.com/google/go-github/v68/github"
)

var (
	// search for these variables
	knSrvPrefix = "knative_serving_version="
	knEvtPrefix = "knative_eventing_version="
	knCtrPrefix = "contour_version="

	file = "hack/component-versions.sh"
)

// if running on branch "release-*" return the current branch version
// (for possible fixups) otherwise, return latest as standard
// func getLatestOrReleaseVersion() (v string, err error) {
// 	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
// 	o, err := cmd.Output()
// 	if err != nil {
// 		return "", fmt.Errorf("error running git rev-parse: %v", err)
// 	}
// 	fmt.Printf("out: %s\n", string(o))
// 	return string(o), nil
// }

// get latest version of owner/repo via GH API
func getLatestVersion(ctx context.Context, client *github.Client, owner string, repo string) (v string, err error) {
	fmt.Printf("get latest repo %s/%s\n", owner, repo)
	rr, res, err := client.Repositories.GetLatestRelease(ctx, owner, repo)
	if err != nil {
		err = fmt.Errorf("error: request for latest %s release: %v", owner+"/"+repo, err)
		return
	}
	if res.StatusCode < 200 && res.StatusCode > 299 {
		err = fmt.Errorf("error: Return status code of request for latest %s release is %d", owner+"/"+repo, res.StatusCode)
		return
	}
	v = *rr.Name
	if v == "" {
		return "", fmt.Errorf("internal error: returned latest release name is empty for '%s'", repo)
	}
	return v, nil
}

// read the allocate.sh file where serving and eventing versions are
// located. Read that file to find them via prefix above. Fetch their version
// and return them in 'v1.23.0' format. (To be compared with the current latest)
func getVersionsFromFile() (srv string, evt string, ctr string, err error) {
	srv = "" //serving
	evt = "" //eventing
	ctr = "" //net-contour (knative-extensions)

	f, err := os.OpenFile(file, os.O_RDWR, 0600)
	if err != nil {
		err = fmt.Errorf("cant open file '%s': %v", file, err)
	}
	defer f.Close()
	// read file line by line
	fs := bufio.NewScanner(f)
	fs.Split(bufio.ScanLines)
	for fs.Scan() {
		// Look for a prefix in a trimmed line.
		line := strings.TrimSpace(fs.Text())
		// Fetch only the version number (after '=' without spaces because bash)
		if strings.HasPrefix(line, knSrvPrefix) {
			srv = strings.Split(line, "=")[1]
			if !strings.HasPrefix(srv, "v") {
				srv = "v" + srv
			}
		} else if strings.HasPrefix(line, knEvtPrefix) {
			evt = strings.Split(line, "=")[1]
			if !strings.HasPrefix(evt, "v") {
				evt = "v" + evt
			}
		} else if strings.HasPrefix(line, knCtrPrefix) {
			ctr = strings.Split(line, "=")[1]
			if !strings.HasPrefix(ctr, "v") {
				ctr = "v" + ctr
			}
		}
		// if all values are acquired, no need to continue
		if srv != "" && evt != "" && ctr != "" {
			break
		}
	}
	return
}

// try updating the version of component named by "repo" via 'sed'
func tryUpdateFile(repo, newV, oldV string) (bool, error) {
	quoteWrap := func(s string) string {
		if !strings.HasPrefix(s, "\"") {
			return "\"" + s + "\""
		}
		return s
	}
	if newV != oldV {
		fmt.Printf("Updating %s from '%s' to '%s'\n", repo, oldV, newV)
		cmd := exec.Command("sed", "-i", "-e", "s/"+quoteWrap(oldV)+"/"+quoteWrap(newV)+"/g", file)
		err := cmd.Run()
		if err != nil {
			return false, fmt.Errorf("error while updating file with '%s' version: %s", repo, err)
		}
		return true, nil
	}
	return false, nil
}

// prepare branch for PR via git commands
func prepareBranch(branchName string) error {
	fmt.Println("> prep branch")
	err := exec.Command("git", "config", "set", "user.email", "\"automation@knative.team\"").Run()
	if err != nil {
		return err
	}
	err = exec.Command("git", "config", "set", "user.name", "\"Knative Automation\"").Run()
	if err != nil {
		return err
	}
	err = exec.Command("git", "switch", "-c", branchName).Run()
	if err != nil {
		return err
	}
	err = exec.Command("git", "add", file).Run()
	if err != nil {
		return err
	}
	err = exec.Command("git", "commit", "-m", "\"update components\"").Run()
	if err != nil {
		return err
	}
	err = exec.Command("git", "push", "origin", branchName, "-f").Run()
	if err != nil {
		return err
	}
	return nil
}

// create a PR for the new updates
func createPR(ctx context.Context, client *github.Client, title string, branchName string) error {
	fmt.Println("> create PR")
	newPR := github.NewPullRequest{
		Title:               github.Ptr(title),
		Base:                github.Ptr("main"),
		Head:                github.Ptr(branchName),
		Body:                github.Ptr(title),
		MaintainerCanModify: github.Ptr(true),
	}
	pr, _, err := client.PullRequests.Create(ctx, "knative", "func", &newPR)

	if err != nil {
		fmt.Printf("err: %s\n", err)
		return err
	}
	fmt.Printf("PR: %#v\n", pr)
	return nil
}

// ----------------------------------------------------------------------------
// ----------------------------------- MAIN -----------------------------------
// ----------------------------------------------------------------------------

// entry function -- essentially "func main() for this file"
func updateComponentVersions() error {
	prTitle := "chore: Update components' versions to latest"
	ctx := context.Background()
	client := github.NewClient(nil).WithAuthToken(os.Getenv("GITHUB_TOKEN"))

	// PR already exists?
	opt := &github.PullRequestListOptions{State: "open"}
	list, _, err := client.PullRequests.List(ctx, "knative", "func", opt)
	if err != nil {
		return fmt.Errorf("errror pulling PRs in knative/func: %s", err)
	}
	for _, pr := range list {
		if pr.GetTitle() == prTitle {
			// gauron99 - cannot update already existing PR, shouldnt happen
			fmt.Printf("PR already exists, exiting\n")
			return nil
		}
	}

	projects := []struct {
		owner, repo string
	}{
		{
			owner: "knative",
			repo:  "serving",
		},
		{
			owner: "knative",
			repo:  "eventing",
		},
		{
			owner: "knative-extensions",
			repo:  "net-contour",
		},
	}

	// get current versions used. Get all together to limit opening/closing
	// the file
	oldSrv, oldEvt, oldCntr, err := getVersionsFromFile()
	if err != nil {
		return err
	}

	updated := false
	// cycle through all versions of components listed above, fetch their
	// latest from github releases - cmp them - create PR for update if necessary
	for _, p := range projects {
		newV, err := getLatestVersion(ctx, client, p.owner, p.repo)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error while getting latest v of %s/%s: %v\n", p.owner, p.repo, err)
			os.Exit(1)
		}

		// sync the old repo & version
		oldV := ""
		switch p.repo {
		case "serving":
			oldV = oldSrv
		case "eventing":
			oldV = oldEvt
		case "net-contour":
			oldV = oldCntr
		}
		// check if component is eligible for update & update if possible
		isNew, err := tryUpdateFile(p.repo, newV, oldV)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return err
		}
		// if any of the files are updated, set this so we create a PR later
		if isNew {
			updated = true
		}
	}

	if !updated {
		// nothing was updated, nothing to do
		fmt.Printf("all good, no newer component releases, exiting\n")
		return nil
	}
	fmt.Printf("file %s updated! Creating a PR...\n", file)
	// create, PR etc etc

	branchName := "update-components" + time.Now().Format(time.DateOnly)
	err = prepareBranch(branchName)
	if err != nil {
		return fmt.Errorf("failed to prep the branch: %v", err)
	}
	err = createPR(ctx, client, prTitle, branchName)
	return err
}
