package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

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

// Update version in file if new releases of any component exist
func tryUpdateFile(upstreams []struct{ owner, repo, version string }) (updated bool, err error) {
	quoteWrap := func(s string) string { return "\"" + s + "\"" }
	file := "hack/component-versions.sh"
	updated = false

	// get current versions used. Get all together to limit opening/closing
	// the file
	oldSrv, oldEvt, oldCntr, err := getVersionsFromFile()
	if err != nil {
		return false, err
	}

	// update files to latest release where applicable
	for _, upstream := range upstreams {
		uv := quoteWrap(upstream.version)
		var cmd *exec.Cmd
		switch upstream.repo {
		case "serving":
			if upstream.version != oldSrv {
				fmt.Printf("update serving from '%s' to '%s'\n", oldSrv, upstream.version)
				cmd = exec.Command("sed", "-i", "-e", "s/"+knSrvPrefix+quoteWrap(oldSrv)+"/"+knSrvPrefix+uv+"/g", file)
			}
		case "eventing":
			if upstream.version != oldEvt {
				fmt.Printf("update eventing from '%s' to '%s'\n", oldEvt, upstream.version)
				cmd = exec.Command("sed", "-i", "-e", "s/"+knEvtPrefix+quoteWrap(oldEvt)+"/"+knEvtPrefix+uv+"/g", file)
			}
		case "net-contour":
			if upstream.version != oldCntr {
				fmt.Printf("update contour from '%s' to '%s'\n", oldCntr, upstream.version)
				cmd = exec.Command("sed", "-i", "-e", "s/"+knCtrPrefix+quoteWrap(oldCntr)+"/"+knCtrPrefix+uv+"/g", file)
			}
		default:
			err = fmt.Errorf("internal error: unknown upstream.repo '%s' in for loop, exiting", upstream.repo)
			return false, err
		}
		err = cmd.Run()
		if err != nil {
			return false, fmt.Errorf("failed to sed %s: %v", upstream.repo, err)
		}
		updated = true
	}

	return updated, nil
}

// entry function -- essentially "func mai(){} for this file"
func updateComponentVersions() error {
	ctx := context.Background()
	client := github.NewClient(nil).WithAuthToken(os.Getenv("GITHUB_TOKEN"))

	// PR already exists?
	// TODO

	projects := []struct {
		owner, repo, version string
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
	var err error
	for i, p := range projects {
		projects[i].version, err = getLatestVersion(ctx, client, p.owner, p.repo)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error while getting latest v of %s/%s: %v\n", p.owner, p.repo, err)
			os.Exit(1)
		}
	}

	updated, err := tryUpdateFile(projects)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}

	if !updated {
		// nothing was updated, nothing to do
		return nil
	}
	// create, PR etc etc
	return nil
}
