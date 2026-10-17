package git

import "testing"

func TestGetRepoOwnerFromGHURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantOwner string
		wantName  string
		wantErr   bool
	}{
		{
			name:      "correct",
			url:       "https://gh/foo/bar",
			wantOwner: "foo",
			wantName:  "bar",
			wantErr:   false,
		},
		{
			name:      "correct with dot git suffix",
			url:       "https://gh/foo/bar.git",
			wantOwner: "foo",
			wantName:  "bar",
			wantErr:   false,
		},
		{
			name:      "correct with capital letters",
			url:       "https://gh/FOO/bar",
			wantOwner: "foo",
			wantName:  "bar",
			wantErr:   false,
		},
		{
			name:    "incorrect url",
			url:     "foobar",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOwner, gotName, err := RepoOwnerAndNameFromUrl(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetRepoOwnerAndNameFromUrl() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotOwner != tt.wantOwner {
				t.Errorf("GetRepoOwnerAndNameFromUrl() gotOwner = %v, wantOwner %v", gotOwner, tt.wantOwner)
			}
			if gotName != tt.wantName {
				t.Errorf("GetRepoOwnerAndNameFromUrl() gotName = %v, wantName %v", gotName, tt.wantName)
			}
		})
	}
}

func TestGitProviderName(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		wantProvider string
		wantErr      bool
	}{
		{
			name:         "GitHub",
			url:          "https://github.com/foo/bar",
			wantProvider: GitHubProvider,
			wantErr:      false,
		},
		{
			name:         "GitLab",
			url:          "https://gitlab.com/foo/bar",
			wantProvider: GitLabProvider,
			wantErr:      false,
		},
		{
			name:         "GitLab repository path contains GitHub",
			url:          "https://gitlab.com/foo/github-actions",
			wantProvider: GitLabProvider,
			wantErr:      false,
		},
		{
			name:         "GitHub repository path contains GitLab",
			url:          "https://github.com/foo/gitlab-runner",
			wantProvider: GitHubProvider,
			wantErr:      false,
		},
		{
			name:         "GitHub SSH",
			url:          "git@github.com:foo/bar.git",
			wantProvider: GitHubProvider,
			wantErr:      false,
		},
		{
			name:         "GitLab SCP repository path contains GitHub",
			url:          "git@gitlab.com:foo/github-actions.git",
			wantProvider: GitLabProvider,
			wantErr:      false,
		},
		{
			name:    "Provider name only in path - not supported",
			url:     "https://example.com/foo/github-actions",
			wantErr: true,
		},
		{
			name:    "Provider name only in username - not supported",
			url:     "https://github@example.com/foo/bar",
			wantErr: true,
		},
		{
			name:    "Provider name is not a complete host label - not supported",
			url:     "https://notgithub.example.com/foo/bar",
			wantErr: true,
		},
		{
			name:         "Uppercase GitLab host",
			url:          "https://GITLAB.COM/foo/bar",
			wantProvider: GitLabProvider,
			wantErr:      false,
		},
		{
			name: "Bitbucket Cloud - not supported",
			url:  "https://bitbucket.com/foo/bar",
			//wantProvider: BitBucketProvider,
			wantErr: true,
		},
		{
			name:         "Foo provider - not supported",
			url:          "https://foo.com/foo/bar",
			wantProvider: "",
			wantErr:      true,
		},
		{
			name:         "Empty URL - not supported",
			url:          "",
			wantProvider: "",
			wantErr:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotProvider, err := GitProviderName(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ProviderGitProviderName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotProvider != tt.wantProvider {
				t.Errorf("ProviderGitProviderName() gotProvider = %v, wantProvider %v", gotProvider, tt.wantProvider)
			}
		})
	}
}
