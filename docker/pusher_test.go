package docker

import "testing"

func Test_parseDigest(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want string
	}{
		{
			name: "basic test",
			arg:  "latest: digest: sha256:a278a91112d17f8bde6b5f802a3317c7c752cf88078dae6f4b5a0784deb81782 size: 2613",
			want: "sha256:a278a91112d17f8bde6b5f802a3317c7c752cf88078dae6f4b5a0784deb81782",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseDigest(tt.arg); got != tt.want {
				t.Errorf("parseDigest() = %v, want %v", got, tt.want)
			}
		})
	}
}
