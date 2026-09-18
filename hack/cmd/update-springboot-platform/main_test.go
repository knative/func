package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestResolveSpringCloudVersion(t *testing.T) {
	mappings := []springCloudMapping{
		{CompatibilityRange: "[3.2.0,3.3.0)", Version: "2023.0.1"},
		{CompatibilityRange: "[3.3.0,3.4.0)", Version: "2023.0.3"},
		{CompatibilityRange: "3.4.0", Version: "2024.0.0"},
	}

	tests := []struct {
		name           string
		springBoot     string
		mappings       []springCloudMapping
		want           string
		wantErrPattern string
	}{
		{
			name:       "matches closed range",
			springBoot: "3.2.5",
			mappings:   mappings,
			want:       "2023.0.1",
		},
		{
			name:       "matches upper closed range boundary exclusively",
			springBoot: "3.3.0",
			mappings:   mappings,
			want:       "2023.0.3",
		},
		{
			name:       "matches open-ended lower bound",
			springBoot: "3.5.0",
			mappings:   mappings,
			want:       "2024.0.0",
		},
		{
			name:           "no compatible mapping",
			springBoot:     "3.0.0",
			mappings:       mappings,
			wantErrPattern: "no compatible spring-cloud version found",
		},
		{
			name:           "unparsable spring boot version",
			springBoot:     "not-a-version",
			mappings:       mappings,
			wantErrPattern: "cannot parse Spring Boot version",
		},
		{
			name:       "malformed range missing comma",
			springBoot: "3.2.5",
			mappings: []springCloudMapping{
				{CompatibilityRange: "[3.2.0]", Version: "bad"},
			},
			wantErrPattern: `cannot parse compatibilityRange "[3.2.0]"`,
		},
		{
			name:       "malformed range unparsable begin",
			springBoot: "3.2.5",
			mappings: []springCloudMapping{
				{CompatibilityRange: "[nope,3.3.0)", Version: "bad"},
			},
			wantErrPattern: "cannot parse begin of compatibilityRange",
		},
		{
			name:       "malformed range unparsable end",
			springBoot: "3.2.5",
			mappings: []springCloudMapping{
				{CompatibilityRange: "[3.2.0,nope)", Version: "bad"},
			},
			wantErrPattern: "cannot parse end of compatibilityRange",
		},
		{
			name:       "malformed open-ended lower bound",
			springBoot: "3.2.5",
			mappings: []springCloudMapping{
				{CompatibilityRange: "nope", Version: "bad"},
			},
			wantErrPattern: "cannot parse compatibilityRange",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSpringCloudVersion(tt.springBoot, tt.mappings)
			if tt.wantErrPattern != "" {
				assert.ErrorContains(t, err, tt.wantErrPattern)
				return
			}
			assert.NilError(t, err)
			assert.Equal(t, got, tt.want)
		})
	}
}

func TestParentVersionFromPOM(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name           string
		content        string
		want           string
		wantErrPattern string
	}{
		{
			name: "finds parent version",
			content: `<project>
	<parent>
		<artifactId>spring-boot-starter-parent</artifactId>
		<version>3.2.5</version>
	</parent>
</project>
`,
			want: "3.2.5",
		},
		{
			name:           "missing parent version",
			content:        `<project></project>`,
			wantErrPattern: "cannot find spring-boot-starter-parent version in",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pomPath := filepath.Join(dir, tt.name+".xml")
			err := os.WriteFile(pomPath, []byte(tt.content), 0644)
			assert.NilError(t, err)

			got, err := parentVersionFromPOM(pomPath)
			if tt.wantErrPattern != "" {
				assert.ErrorContains(t, err, tt.wantErrPattern)
				return
			}
			assert.NilError(t, err)
			assert.Equal(t, got, tt.want)
		})
	}
}

func TestUpdatePOM(t *testing.T) {
	dir := t.TempDir()

	t.Run("updates both parent and spring-cloud versions", func(t *testing.T) {
		pomPath := filepath.Join(dir, "pom.xml")
		err := os.WriteFile(pomPath, []byte(`<project>
	<parent>
		<artifactId>spring-boot-starter-parent</artifactId>
		<version>3.2.5</version>
	</parent>
	<properties>
		<spring-cloud.version>2023.0.1</spring-cloud.version>
	</properties>
</project>
`), 0644)
		assert.NilError(t, err)

		err = updatePOM(pomPath, "3.3.0", "2023.0.3")
		assert.NilError(t, err)

		data, err := os.ReadFile(pomPath)
		assert.NilError(t, err)
		assert.Assert(t, strings.Contains(string(data), "<version>3.3.0</version>"))
		assert.Assert(t, strings.Contains(string(data), "<spring-cloud.version>2023.0.3</spring-cloud.version>"))
	})

	t.Run("errors when spring-cloud.version tag is absent", func(t *testing.T) {
		pomPath := filepath.Join(dir, "no-spring-cloud.xml")
		err := os.WriteFile(pomPath, []byte(`<project>
	<parent>
		<artifactId>spring-boot-starter-parent</artifactId>
		<version>3.2.5</version>
	</parent>
</project>
`), 0644)
		assert.NilError(t, err)

		err = updatePOM(pomPath, "3.3.0", "2023.0.3")
		assert.ErrorContains(t, err, "cannot find <spring-cloud.version>")

		// the parent version must be left untouched when we bail out early.
		data, err := os.ReadFile(pomPath)
		assert.NilError(t, err)
		assert.Assert(t, strings.Contains(string(data), "<version>3.2.5</version>"))
	})
}
