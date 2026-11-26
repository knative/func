package main

import (
	"fmt"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

// mavenMetadataXML is the maven-metadata.xml of an artifact with versions.
func mavenMetadataXML(versions ...string) string {
	var b strings.Builder
	b.WriteString("<metadata><versioning><versions>")
	for _, v := range versions {
		fmt.Fprintf(&b, "<version>%s</version>", v)
	}
	b.WriteString("</versions></versioning></metadata>")
	return b.String()
}

// updateQuarkus moves the templates to the newest release of their major
// version.
func TestUpdateQuarkus(t *testing.T) {
	quarkus := serve(t, map[string]string{
		"/api/platforms": `{"platforms": [{"streams": [
			{"releases": [{"version": "4.0.1"}]},
			{"releases": [{"version": "3.40.1.2"}]},
			{"releases": [{"version": "3.39.6"}]}
		]}]}`,
	})
	t.Chdir(t.TempDir())
	writeTemplates(t, quarkusPomPaths, quarkusPom)

	assert.NilError(t, sources{quarkus: quarkus.URL + "/api/platforms"}.updateQuarkus(t.Context()))

	assertFiles(t, quarkusPomPaths, strings.Replace(quarkusPom, "3.39.5", "3.40.1.2", 1))
}

// updateSpringBoot moves the templates to the newest patch of their Spring
// Boot minor version and of their Spring Cloud release train.
func TestUpdateSpringBoot(t *testing.T) {
	maven := serve(t, map[string]string{
		"/maven2/org/springframework/boot/spring-boot-starter-parent/maven-metadata.xml": mavenMetadataXML("3.5.12", "3.5.16", "3.6.0", "4.0.0"),
		"/maven2/org/springframework/cloud/spring-cloud-dependencies/maven-metadata.xml": mavenMetadataXML("2025.0.1", "2025.0.3", "2025.1.0", "2026.0.0"),
	})
	t.Chdir(t.TempDir())
	writeTemplates(t, springBootPomPaths, springBootPom)

	assert.NilError(t, sources{maven: maven.URL + "/maven2"}.updateSpringBoot(t.Context()))

	want := strings.Replace(springBootPom, "<version>3.5.12</version>", "<version>3.5.16</version>", 1)
	want = strings.Replace(want, "2025.0.1", "2025.0.3", 1)
	assertFiles(t, springBootPomPaths, want)
}

// A source that answers with something else than the release list is an
// error.
func TestVersionsBadAnswer(t *testing.T) {
	srv := serve(t, map[string]string{
		"/maven2/io/example/maven-metadata.xml": "<metadata><versioning>",
		"/api/platforms":                        "not JSON",
	})

	_, err := mavenVersions(t.Context(), srv.URL+"/maven2", "io/example")
	assert.Assert(t, err != nil)
	_, err = quarkusVersions(t.Context(), srv.URL+"/api/platforms")
	assert.Assert(t, err != nil)
	_, err = mavenVersions(t.Context(), srv.URL+"/maven2", "io/missing")
	assert.ErrorContains(t, err, "returned status 404")
	_, err = quarkusVersions(t.Context(), srv.URL+"/api/missing")
	assert.ErrorContains(t, err, "returned status 404")
}

func TestNewest(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		versions []string
		keep     int
		want     string
	}{
		{
			name:     "quarkus moves to newest minor of same major",
			current:  "3.39.5",
			versions: []string{"3.40.1.1", "3.33.4.1", "3.27.6.1", "4.0.0.Beta2"},
			keep:     1,
			want:     "3.40.1.1",
		},
		{
			name:     "quarkus skips final release of next major",
			current:  "3.40.1.1",
			versions: []string{"4.0.1", "3.40.1.1", "3.33.4.1"},
			keep:     1,
			want:     "3.40.1.1",
		},
		{
			name:     "spring boot stays on its minor",
			current:  "3.5.12",
			versions: []string{"3.5.11", "3.5.16", "3.5.13", "3.6.0", "4.1.2"},
			keep:     2,
			want:     "3.5.16",
		},
		{
			name:     "spring cloud stays on its train",
			current:  "2025.0.1",
			versions: []string{"2025.0.0", "2025.0.3", "2025.0.4-RC1", "2025.1.4", "Hoxton.SR12"},
			keep:     2,
			want:     "2025.0.3",
		},
		{
			name:     "pre-releases are skipped",
			current:  "3.5.12",
			versions: []string{"3.5.12", "3.5.13-M1", "3.5.13-RC1", "3.5.13-SNAPSHOT"},
			keep:     2,
			want:     "3.5.12",
		},
		{
			name:     "never downgrades",
			current:  "3.5.12",
			versions: []string{"3.5.10", "3.5.11"},
			keep:     2,
			want:     "3.5.12",
		},
		{
			name:     "compares numbers, not strings",
			current:  "3.5.9",
			versions: []string{"3.5.10"},
			keep:     2,
			want:     "3.5.10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newest(tt.current, tt.versions, tt.keep)
			assert.NilError(t, err)
			assert.Equal(t, got, tt.want)
		})
	}
}

func TestNewestBadCurrent(t *testing.T) {
	for _, current := range []string{"", "3.5.x", "${spring-boot.version}", "3"} {
		_, err := newest(current, []string{"3.5.16"}, 2)
		assert.ErrorContains(t, err, "cannot parse version")
	}
}

// A source that lists no release, such as an API that changed its format,
// is an error rather than "up-to-date".
func TestNewestNoReleases(t *testing.T) {
	for _, versions := range [][]string{nil, {""}, {"3.5.13-RC1", "Hoxton.SR12"}} {
		_, err := newest("3.5.12", versions, 2)
		assert.Error(t, err, "the source lists no releases", "versions %q", versions)
	}
}
