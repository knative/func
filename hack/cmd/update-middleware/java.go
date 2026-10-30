package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var (
	quarkusPomPaths = []string{
		"templates/quarkus/cloudevents/pom.xml",
		"templates/quarkus/http/pom.xml",
	}
	springBootPomPaths = []string{
		"templates/springboot/cloudevents/pom.xml",
		"templates/springboot/http/pom.xml",
	}

	quarkusVersionRe     = regexp.MustCompile(`<quarkus\.platform\.version>([^<]+)</quarkus\.platform\.version>`)
	springBootVersionRe  = regexp.MustCompile(`<artifactId>spring-boot-starter-parent</artifactId>\s*<version>([^<]+)</version>`)
	springCloudVersionRe = regexp.MustCompile(`<spring-cloud\.version>([^<]+)</spring-cloud\.version>`)
)

type quarkusPlatforms struct {
	Platforms []struct {
		Streams []struct {
			Releases []struct {
				Version string `json:"version"`
			} `json:"releases"`
		} `json:"streams"`
	} `json:"platforms"`
}

type mavenMetadata struct {
	Versions []string `xml:"versioning>versions>version"`
}

// updateQuarkus moves the Quarkus platform to the newest final release of
// its current major version.
func (s sources) updateQuarkus(ctx context.Context) error {
	versions, err := quarkusVersions(ctx, s.quarkus)
	if err != nil {
		return fmt.Errorf("cannot get Quarkus versions: %w", err)
	}

	if err := updatePoms(quarkusPomPaths, quarkusVersionRe, versions, 1); err != nil {
		return fmt.Errorf("cannot update Quarkus: %w", err)
	}
	return nil
}

// updateSpringBoot moves Spring Boot to the newest patch of its current minor
// version and Spring Cloud to the newest patch of its current release train.
func (s sources) updateSpringBoot(ctx context.Context) error {
	springBoot, err := mavenVersions(ctx, s.maven, "org/springframework/boot/spring-boot-starter-parent")
	if err != nil {
		return fmt.Errorf("cannot get Spring Boot versions: %w", err)
	}
	springCloud, err := mavenVersions(ctx, s.maven, "org/springframework/cloud/spring-cloud-dependencies")
	if err != nil {
		return fmt.Errorf("cannot get Spring Cloud versions: %w", err)
	}

	if err := updatePoms(springBootPomPaths, springBootVersionRe, springBoot, 2); err != nil {
		return fmt.Errorf("cannot update Spring Boot: %w", err)
	}
	if err := updatePoms(springBootPomPaths, springCloudVersionRe, springCloud, 2); err != nil {
		return fmt.Errorf("cannot update Spring Cloud: %w", err)
	}
	return nil
}

// updatePoms sets the version that re captures in the poms at paths to the
// newest of versions that shares its first keep parts.
func updatePoms(paths []string, re *regexp.Regexp, versions []string, keep int) error {
	current, err := readVersion(paths, re)
	if err != nil {
		return err
	}
	latest, err := newest(current, versions, keep)
	if err != nil {
		return err
	}
	return updateFiles(paths, re, latest)
}

// newest returns the newest release in versions that shares its first keep
// parts with current, or current if none is newer. It skips versions that
// are not all numbers, such as betas, milestones and snapshots. A list with
// no release at all is an error, as it means the source is broken.
func newest(current string, versions []string, keep int) (string, error) {
	best, ok := parseVersion(current)
	if !ok || len(best) < keep {
		return "", fmt.Errorf("cannot parse version %q", current)
	}
	line := best[:keep]

	result, listed := current, false
	for _, v := range versions {
		p, ok := parseVersion(v)
		if !ok {
			continue
		}
		listed = true
		if len(p) < keep || !slices.Equal(p[:keep], line) || slices.Compare(p, best) <= 0 {
			continue
		}
		best, result = p, v
	}
	if !listed {
		return "", errors.New("the source lists no releases")
	}
	return result, nil
}

// parseVersion splits a dotted version such as "3.40.1.1" into numbers.
func parseVersion(s string) ([]int, bool) {
	parts := strings.Split(s, ".")
	nums := make([]int, len(parts))
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, false
		}
		nums[i] = n
	}
	return nums, true
}

// quarkusVersions lists the Quarkus platform releases that the
// code.quarkus.io platforms API at url names.
func quarkusVersions(ctx context.Context, url string) ([]string, error) {
	body, err := get(ctx, url)
	if err != nil {
		return nil, err
	}

	var platforms quarkusPlatforms
	if err := json.Unmarshal(body, &platforms); err != nil {
		return nil, err
	}

	var versions []string
	for _, platform := range platforms.Platforms {
		for _, stream := range platform.Streams {
			for _, release := range stream.Releases {
				versions = append(versions, release.Version)
			}
		}
	}
	return versions, nil
}

// mavenVersions lists the versions of the artifact at path, such as
// "org/example/example-bom", in the Maven repository at repo.
func mavenVersions(ctx context.Context, repo, path string) ([]string, error) {
	body, err := get(ctx, repo+"/"+path+"/maven-metadata.xml")
	if err != nil {
		return nil, err
	}

	var metadata mavenMetadata
	if err := xml.Unmarshal(body, &metadata); err != nil {
		return nil, err
	}
	return metadata.Versions, nil
}
