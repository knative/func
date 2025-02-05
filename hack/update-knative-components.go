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

// get latest version of owner/repo via GH API
func getLatestVersion(ctx context.Context, client *github.Client, owner string, repo string) (v string, err error) {
	fmt.Printf("> get latest '%s/%s'...", owner, repo)
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
	fmt.Println("done")
	return v, nil
}

// Read the file where components versions are located. Fetch their version
// and return them in 'v1.23.0' format (unquoted).
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
		// trim white space -> split the line via '='
		lineArr := strings.Split(strings.TrimSpace(fs.Text()), "=")
		if len(lineArr) != 2 {
			continue
		}
		// add "=" for consise matching of the value assignment -- in case the value
		// is used elsewhere in the file for any reason.
		prefix := lineArr[0] + "="
		// trim '"' and 'v' from the value for robustness (will be added back later)
		// "v1.2.3" or v1.2.3 or "1.2.3" or v1.2.3 --> 1.2.3
		val := strings.TrimFunc(lineArr[1], func(r rune) bool {
			return r == '"' || r == 'v'
		})

		switch prefix {
		case knSrvPrefix:
			srv = "v" + val
		case knEvtPrefix:
			evt = "v" + val
		case knCtrPrefix:
			ctr = "v" + val
		}

		// if all values are acquired, no need to continue
		if srv != "" && evt != "" && ctr != "" {
			break
		}
	}
	return
}

// try updating the version of component named by "repo" via 'sed'
func tryUpdateFile(prefix, newV, oldV string) (bool, error) {
	fmt.Printf("> try updating %s(%s -> %s)...", prefix, oldV, newV)
	quoteWrap := func(s string) string {
		if !strings.HasPrefix(s, "\"") {
			return "\"" + s + "\""
		}
		return s
	}
	if newV != oldV {
		fmt.Println("updating")
		cmd := exec.Command("sed", "-i", "-e", "s/"+prefix+quoteWrap(oldV)+"/"+prefix+quoteWrap(newV)+"/g", file)
		err := cmd.Run()
		if err != nil {
			return false, fmt.Errorf("error while updating file: %s", err)
		}
		return true, nil
	}
	return false, nil
}

// prepare branch for PR via git commands
func prepareBranch(branchName string) error {
	fmt.Println("> preparing branch")
	cmd := exec.Command("bash", "-c", fmt.Sprintf(`
		git config --local user.email "automation@knative.team" &&
		git config --local user.name "Knative Automation" &&
		git switch -c %s &&
		git add %s &&
		git commit -m "update components" &&
		git push origin %s
	`, branchName, file, branchName))
	return cmd.Run()
}

// create a PR for the new updates
func createPR(ctx context.Context, client *github.Client, title string, branchName string) error {
	fmt.Print("> creating PR...")
	body := fmt.Sprintf("%s\n/assign @gauron99", title)

	newPR := github.NewPullRequest{
		Title:               github.Ptr(title),
		Base:                github.Ptr("main"),
		Head:                github.Ptr(branchName),
		Body:                github.Ptr(body),
		MaintainerCanModify: github.Ptr(true),
	}
	pr, _, err := client.PullRequests.Create(ctx, "knative", "func", &newPR)
	if err != nil {
		fmt.Printf("PR looks like this:\n%#v\n", pr)
		fmt.Printf("err: %s\n", err)
		return err
	}
	fmt.Println("ready")
	return nil
}

// returns true when PR with given title already exists in knative/func repo
// otherwise false. Returns an error if occured, otherwise nil.
func prExists(ctx context.Context, c *github.Client, title string) (bool, error) {
	opt := &github.PullRequestListOptions{State: "open"}
	list, _, err := c.PullRequests.List(ctx, "knative", "func", opt)
	if err != nil {
		return false, fmt.Errorf("errror pulling PRs in knative/func: %s", err)
	}
	for _, pr := range list {
		if pr.GetTitle() == title {
			// gauron99 - currently cannot update already existing PR, shouldnt happen
			return true, nil
		}
	}
	return false, nil
}

// ----------------------------------------------------------------------------
// ----------------------------------- MAIN -----------------------------------
// ----------------------------------------------------------------------------

// entry function -- essentially "func main() for this file"
func updateComponentVersions() error {
	prTitle := "chore: Update components' versions to latest"
	ctx := context.Background()
	client := github.NewClient(nil).WithAuthToken(os.Getenv("GITHUB_TOKEN"))

	e, err := prExists(ctx, client, prTitle)
	if err != nil {
		return err
	}
	if e {
		fmt.Printf("PR already exists, nothing to do, exiting")
		return nil
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

	// Get current versions used.
	// Get all together to limit opening/closingthe file.
	// Could be reworked to keep the file open through out the for cycle.
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
			return fmt.Errorf("error while getting latest v of %s/%s: %v", p.owner, p.repo, err)
		}

		// sync the old repo & version
		oldV := ""
		prefix := ""
		switch p.repo {
		case "serving":
			oldV = oldSrv
			prefix = knSrvPrefix
		case "eventing":
			oldV = oldEvt
			prefix = knEvtPrefix
		case "net-contour":
			oldV = oldCntr
			prefix = knCtrPrefix
		}
		// try and overwrite the file with new versions
		isNew, err := tryUpdateFile(prefix, newV, oldV)
		if err != nil {
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
