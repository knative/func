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
