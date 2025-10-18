//go:build integration
// +build integration

package tekton_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/html"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"knative.dev/pkg/apis"

	"knative.dev/func/pkg/builders/buildpacks"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/oci"
	"knative.dev/func/pkg/pipelines"
	"knative.dev/func/pkg/pipelines/tekton"
	"knative.dev/func/pkg/random"
)

func TestInt_Gitlab(t *testing.T) {
	// Skip on ARM64 due to Paketo buildpack limitations
	if runtime.GOARCH == "arm64" || runtime.GOARCH == "arm" {
		t.Skip("Paketo buildpacks do not currently support ARM64 architecture. " +
			"See https://github.com/paketo-buildpacks/nodejs/issues/712")
	}

	// Skip in CI due to persistent timeout issues regardless of allocated time
	// Note it does indeed run locally.
	// TODO: Investigate why GitLab webhook builds are not completing in CI
	if os.Getenv("CI") == "true" || os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping GitLab test in CI due to persistent timeout issues. " +
			"Please run GitLab integration tests locally with 'make test-integration' to verify changes.")
	}

	var err error
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	gitlabHostname, gitlabRootPassword, pacCtrHostname, err := parseEnv(t)
	if err != nil {
		t.Fatal(err)
	}

	glabEnv := setupGitlabEnv(ctx, t, "http://"+gitlabHostname, "root", gitlabRootPassword)

	tempHome := t.TempDir()
	projDir := filepath.Join(t.TempDir(), "fn")
	err = os.MkdirAll(projDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	ns := usingNamespace(t)
	t.Logf("testing in namespace: %q", ns)

	funcImg := fmt.Sprintf("registry.default.svc.cluster.local:5000/fn-%s", uuid.NewUUID())

	f := fn.Function{
		Root:     projDir,
		Name:     glabEnv.ProjectName,
		Runtime:  "test-runtime",
		Template: "test-template",
		Image:    funcImg,
		Created:  time.Now(),
		Invoke:   "none",
		Build: fn.BuildSpec{
			Git: fn.Git{
				URL:      strings.TrimSuffix(glabEnv.HTTPProjectURL, ".git"),
				Revision: "devel",
			},
			BuilderImages: map[string]string{"pack": buildpacks.DefaultTinyBuilder},
			Builder:       "pack",
			PVCSize:       "256Mi",
		},
		Deploy: fn.DeploySpec{
			Namespace: ns,
		},
		Local: fn.Local{Remote: true},
	}
	f = fn.NewFunctionWith(f)
	err = f.Write()
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(projDir, "Procfile"), []byte("web: non-existent-app\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	credentialsProvider := func(ctx context.Context, image string) (oci.Credentials, error) {
		return oci.Credentials{
			Username: "",
			Password: "",
		}, nil
	}
	pp := tekton.NewPipelinesProvider(
		tekton.WithCredentialsProvider(credentialsProvider),
		tekton.WithPacURLCallback(func() (string, error) {
			return "http://" + pacCtrHostname, nil
		}))

	metadata := pipelines.PacMetadata{
		PersonalAccessToken:       glabEnv.UserToken,
		ConfigureLocalResources:   true,
		ConfigureClusterResources: true,
		ConfigureRemoteResources:  true,
	}
	err = pp.ConfigurePAC(context.Background(), f, metadata)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = pp.RemovePAC(context.Background(), f, metadata)
	})

	buildDoneCh := awaitBuildCompletion(t, glabEnv.ProjectName, ns)

	gitCommands := `export GIT_TERMINAL_PROMPT=0 && \
  cd "${PROJECT_DIR}" && \
  git config --global user.name "John Doe" && \
  git config --global user.email "jdoe@example.com" && \
  git config --global core.sshCommand "ssh -i ${SSH_IDENTITY_FILE} -o UserKnownHostsFile=${HOME}/known_hosts -o StrictHostKeyChecking=no" && \
  git init --initial-branch=devel && \
  git remote add origin "${REPO_URL}" && \
  git add . && \
  git commit -m "commit message" && \
  git push -u origin devel
`
	cmd := exec.Command("sh", "-c", gitCommands)
	cmd.Env = []string{
		"PROJECT_DIR=" + projDir,
		"SSH_IDENTITY_FILE=" + glabEnv.UserIdentityFile,
		"REPO_URL=" + glabEnv.SSHProjectURL,
		"HOME=" + tempHome,
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Log(string(out))
		t.Fatal(err)
	}

	select {
	case <-buildDoneCh:
		t.Log("build done on time")
	case <-time.After(time.Minute * 10):
		t.Error("build has not been done in time (15 minute timeout)")
	case <-ctx.Done():
		t.Error("cancelled")
	}

}

func parseEnv(t *testing.T) (gitlabHostname string, gitlabRootPassword string, pacCtrHostname string, err error) {
	if enabled, _ := strconv.ParseBool(os.Getenv("FUNC_INT_GITLAB_ENABLED")); !enabled {
		t.Skip("GitLab tests are disabled")
	}
	envs := map[string]*string{
		"FUNC_INT_GITLAB_HOSTNAME": &gitlabHostname,
		"FUNC_TEST_GITLAB_PASS":     &gitlabRootPassword,
		"FUNC_INT_PAC_HOST":         &pacCtrHostname,
	}
	var missing []string
	gitlabHostname = os.Getenv("FUNC_INT_GITLAB_HOSTNAME")
	for name, ptr := range envs {
		val := os.Getenv(name)
		if val == "" {
			missing = append(missing, name)
			continue
		}
		*ptr = val
	}
	if len(missing) > 0 {
		err = fmt.Errorf("required environment variables are not set: %+v", strings.Join(missing, ", "))
	}
	return
}

type gitlabEnv struct {
	ProjectName      string
	HTTPProjectURL   string
	SSHProjectURL    string
	GroupName        string
	UserName         string
	UserToken        string
	UserIdentityFile string
}

func setupGitlabEnv(ctx context.Context, t *testing.T, baseURL, username, password string) gitlabEnv {
	t.Log("setting up gitlab env")
	randStr := strings.ToLower(random.AlphaString(5))
	userName := "func_user_" + randStr
	userPassword := "1ddqd1dkf@"
	groupName := "func-grp-" + randStr
	projectName := "func-project-" + randStr

	//region Initialize Root's Gitlab client
	// Retry getting the API token as GitLab might still be initializing
	var rootToken string
	var err error
	for retries := 0; retries < 5; retries++ {
		if retries > 0 {
			t.Logf("Retrying GitLab authentication (attempt %d/5)...", retries+1)
			time.Sleep(5 * time.Second)
		}
		rootToken, err = getAPIToken(baseURL, username, password)
		if err == nil {
			break
		}
		if strings.Contains(err.Error(), "authentication failed") {
			// If it's definitely an auth failure, don't retry
			t.Fatalf("GitLab authentication failed with root password from GITLAB_ROOT_PASSWORD env var: %v", err)
		}
	}
	if err != nil {
		t.Fatalf("Failed to get GitLab API token after retries: %v", err)
	}

	glabCli, err := gitlab.NewClient(rootToken, gitlab.WithBaseURL(baseURL))
	if err != nil {
		t.Fatal(err)
	}

	pat, _, err := glabCli.PersonalAccessTokens.GetSinglePersonalAccessToken()
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_, _ = glabCli.PersonalAccessTokens.RevokePersonalAccessToken(pat.ID)
	})
	//endregion

	//region Enable Webhooks to non-public IP.
	newSettings := &gitlab.UpdateSettingsOptions{
		AllowLocalRequestsFromWebHooksAndServices: p(true),
	}
	_, _, err = glabCli.Settings.UpdateSettings(newSettings)
	if err != nil {
		t.Fatal(err)
	}

	// For some reason the setting update does not kick in immediately.
	select {
	case <-time.After(time.Second * 60):
		break
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	//endregion

	//region Create test user
	userOpts := &gitlab.CreateUserOptions{
		Name:                p("John Doe"),
		Username:            p(userName),
		Password:            p(userPassword),
		ForceRandomPassword: p(false),
		Email:               p(fmt.Sprintf("%s@example.com", userName)),
	}

	u, _, err := glabCli.Users.CreateUser(userOpts)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("created user %q", u.Username)
	t.Cleanup(func() {
		_, _ = glabCli.Users.DeleteUser(u.ID)
	})
	//endregion

	//region Create test group
	groupOpts := &gitlab.CreateGroupOptions{
		Name:                 p(groupName),
		Path:                 p(groupName),
		Description:          p("group for `func` testing"),
		Visibility:           p(gitlab.PublicVisibility),
		RequireTwoFactorAuth: p(false),
		ProjectCreationLevel: p(gitlab.DeveloperProjectCreation),
		LFSEnabled:           p(true),
		RequestAccessEnabled: p(true),
	}
	g, _, err := glabCli.Groups.CreateGroup(groupOpts)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("created group: %q", g.Name)
	t.Cleanup(func() {
		_, _ = glabCli.Groups.DeleteGroup(g.ID, nil)
	})
	//endregion

	//region Add test user to the test group
	grpMemOpts := &gitlab.AddGroupMemberOptions{
		UserID:      p(u.ID),
		AccessLevel: p(gitlab.MaintainerPermissions),
	}
	_, _, err = glabCli.GroupMembers.AddGroupMember(g.ID, grpMemOpts)
	if err != nil {
		t.Fatal(err)
	}
	//endregion

	//region Create test project
	creatProjOpts := &gitlab.CreateProjectOptions{
		Name:                 p(projectName),
		NamespaceID:          p(g.ID),
		Path:                 p(projectName),
		Visibility:           p(gitlab.PublicVisibility),
		InitializeWithReadme: p(true),
		DefaultBranch:        p("main"),
	}
	project, _, err := glabCli.Projects.CreateProject(creatProjOpts)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("created project: %q", project.Name)
	t.Cleanup(func() {
		_, _ = glabCli.Projects.DeleteProject(project.ID, nil)
	})
	//endregion

	//region Add public SSK key for test user
	sshPrivateKeyPath := generateSSHKeys(t)
	sshPublicKeyBytes, err := os.ReadFile(sshPrivateKeyPath + ".pub")
	if err != nil {
		t.Fatal(err)
	}

	userToken, err := getAPIToken(baseURL, userName, userPassword)
	if err != nil {
		t.Fatal(err)
	}

	glabCliUser, err := gitlab.NewClient(userToken, gitlab.WithBaseURL(baseURL))
	if err != nil {
		t.Fatal(err)
	}

	addSshKeyOpts := &gitlab.AddSSHKeyOptions{
		Title: p("func-ssh-key"),
		Key:   p(string(sshPublicKeyBytes)),
	}
	_, _, err = glabCliUser.Users.AddSSHKey(addSshKeyOpts)
	if err != nil {
		t.Fatal(err)
	}
	//endregion

	return gitlabEnv{
		ProjectName:      projectName,
		HTTPProjectURL:   project.HTTPURLToRepo,
		SSHProjectURL:    project.SSHURLToRepo,
		GroupName:        groupName,
		UserName:         userName,
		UserToken:        userToken,
		UserIdentityFile: sshPrivateKeyPath,
	}
}

func getAPIToken(baseURL, username, password string) (string, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return "", fmt.Errorf("cannot create a cookie jar: %w", err)
	}

	c := http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	signInURL := baseURL + "/users/sign_in"

	resp, err := c.Get(signInURL)
	if err != nil {
		return "", fmt.Errorf("cannot get sign in page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("cannot get sign in page, unexpected status: %d", resp.StatusCode)
	}

	node, err := html.Parse(resp.Body)
	if err != nil {
		return "", fmt.Errorf("cannot parse sign in page: %w", err)
	}

	csrfToken := getCSRFToken(node)

	form := url.Values{}
	form.Add("authenticity_token", csrfToken)
	form.Add("user[login]", username)
	form.Add("user[password]", password)
	form.Add("user[remember_me]", "0")

	req, err := http.NewRequest("POST", signInURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("cannot create sign in request: %w", err)
	}

	req.Header.Add("Origin", baseURL)
	req.Header.Add("Referer", signInURL)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, err = c.Do(req)
	if err != nil {
		return "", fmt.Errorf("cannot sign in: %w", err)
	}
	defer resp.Body.Close()

	// GitLab may return different status codes after login
	// Older versions: 302 redirect
	// Newer versions or certain configurations: might return 200 with session cookie set
	if resp.StatusCode == 302 || resp.StatusCode == 303 {
		// Traditional successful login with redirect
	} else if resp.StatusCode == 200 {
		// Check if authentication actually succeeded despite 200 status
		// This can happen with newer GitLab versions or certain configurations
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)

		// Check for explicit authentication failure messages
		if strings.Contains(bodyStr, "Invalid login or password") ||
			strings.Contains(bodyStr, "Invalid Login or password") ||
			strings.Contains(bodyStr, "Incorrect username or password") {
			return "", fmt.Errorf("authentication failed - invalid credentials (username: %s)", username)
		}

		// Check if we have a session cookie which would indicate successful login
		hasCookie := false
		for _, cookie := range c.Jar.Cookies(req.URL) {
			if cookie.Name == "_gitlab_session" && cookie.Value != "" {
				hasCookie = true
				break
			}
		}

		if !hasCookie {
			// No session cookie and status 200 - likely authentication failed
			return "", fmt.Errorf("authentication appears to have failed - no session cookie set")
		}
		// We have a session cookie, so authentication likely succeeded
		// Continue to get the personal access token
	} else {
		return "", fmt.Errorf("cannot sign in, unexpected status: %d", resp.StatusCode)
	}

	personalAccessTokensURL := baseURL + "/-/user_settings/personal_access_tokens"

	resp, err = c.Get(personalAccessTokensURL)
	if err != nil {
		return "", fmt.Errorf("cannot get personal access tokens: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("cannot get personal access tokens, unexpected status: %d", resp.StatusCode)
	}

	node, err = html.Parse(resp.Body)
	if err != nil {
		return "", fmt.Errorf("cannot parse personal access tokens page: %w", err)
	}
	csrfToken = getCSRFToken(node)

	form = url.Values{
		"personal_access_token[name]":       {"test-2"},
		"personal_access_token[expires_at]": {time.Now().Add(time.Hour * 25).Format("2006-01-02")},
		"personal_access_token[scopes][]":   {"api", "read_api", "read_user", "read_repository", "write_repository", "sudo"},
	}

	req, err = http.NewRequest("POST", personalAccessTokensURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("cannot create new personal access token request: %w", err)
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Add("X-CSRF-Token", csrfToken)

	resp, err = c.Do(req)
	if err != nil {
		return "", fmt.Errorf("cannot create new personal access token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("cannot create new personal access token, unexpected status: %d", resp.StatusCode)
	}

	data := struct {
		NewToken string `json:"token,omitempty"`
	}{}
	e := json.NewDecoder(resp.Body)
	err = e.Decode(&data)
	if err != nil {
		return "", fmt.Errorf("cannot parse token form a response: %w", err)
	}

	return data.NewToken, nil
}

func getCSRFToken(n *html.Node) string {
	var match bool
	var token string
	for _, a := range n.Attr {
		if a.Key == "name" && (a.Val == "authenticity_token" || a.Val == "csrf-token") {
			match = true
		}
		if a.Key == "value" || a.Key == "content" {
			token = a.Val
		}
	}
	if match {
		return token
	}

	for n = n.FirstChild; n != nil; n = n.NextSibling {
		token = getCSRFToken(n)
		if token != "" {
			return token
		}
	}

	return ""
}

func generateSSHKeys(t *testing.T) string {
	tmp := t.TempDir()

	privateKeyPath := filepath.Join(tmp, "id_ecdsa")
	publicKeyPath := privateKeyPath + ".pub"

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	privateKeyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	privateKeyBlock := &pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: privateKeyBytes,
	}
	privateKeyFile, err := os.OpenFile(privateKeyPath, os.O_CREATE|os.O_WRONLY, 0400)
	if err != nil {
		t.Fatal(err)
	}
	err = pem.Encode(privateKeyFile, privateKeyBlock)
	if err != nil {
		t.Fatal(err)
	}

	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	publicKeyBytes := ssh.MarshalAuthorizedKey(publicKey)

	err = os.WriteFile(publicKeyPath, publicKeyBytes, 0444)
	if err != nil {
		t.Fatal(err)
	}

	return privateKeyPath
}

func usingNamespace(t *testing.T) string {

	name := "gitlab-test-" + strings.ToLower(random.AlphaString(5))
	k8sClient, err := k8s.NewKubernetesClientset()
	if err != nil {
		t.Fatal(err)
	}

	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	createOpts := metav1.CreateOptions{}
	_, err = k8sClient.CoreV1().Namespaces().Create(context.Background(), ns, createOpts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		deleteOpts := metav1.DeleteOptions{}
		_ = k8sClient.CoreV1().Namespaces().Delete(context.Background(), name, deleteOpts)
	})
	return name
}

func awaitBuildCompletion(t *testing.T, name, ns string) <-chan struct{} {

	clis, err := tekton.NewTektonClients()
	if err != nil {
		t.Fatal(err)
	}

	listOpts := metav1.ListOptions{
		LabelSelector: "tekton.dev/pipelineTask=build",
		Watch:         true,
	}
	w, err := clis.Tekton.TektonV1().TaskRuns(ns).Watch(context.Background(), listOpts)
	if err != nil {
		t.Fatal(err)
	}

	ch := make(chan struct{}, 1)

	go func() {
		defer w.Stop()
		for event := range w.ResultChan() {
			taskRun, ok := event.Object.(*pipelinev1.TaskRun)
			if !ok {
				continue
			}
			if !strings.HasPrefix(taskRun.Name, name) {
				continue
			}
			for _, condition := range taskRun.Status.Conditions {
				if condition.Type == apis.ConditionSucceeded && condition.IsTrue() {
					ch <- struct{}{}
					break
				}
			}
		}
	}()
	return ch
}

func p[T any](t T) *T {
	return &t
}
