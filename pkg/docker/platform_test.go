package docker_test

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"knative.dev/func/pkg/docker"
)

func TestPlatform(t *testing.T) {
	testRegistry := startRegistry(t)

	nonMultiArchBuilder := testRegistry + "/default/builder:nonmultiarch"
	multiArchBuilder := testRegistry + "/default/builder:multiarch"

	// begin push testing builders to registry
	tag, err := name.NewTag(nonMultiArchBuilder)
	if err != nil {
		t.Fatal(err)
	}

	var img v1.Image
	img, err = mutate.ConfigFile(empty.Image, &v1.ConfigFile{
		Architecture: "ppc64le",
		OS:           "linux",
	})
	if err != nil {
		t.Fatal(err)
	}

	err = remote.Write(&tag, img)
	if err != nil {
		t.Fatal(err)
	}

	tag, err = name.NewTag(multiArchBuilder)
	if err != nil {
		t.Fatal(err)
	}

	var imgIdx = mutate.AppendManifests(empty.Index, mutate.IndexAddendum{
		Add: img,
		Descriptor: v1.Descriptor{
			Platform: &v1.Platform{
				Architecture: "ppc64le",
				OS:           "linux",
			},
		},
	})

	err = remote.WriteIndex(tag, imgIdx)
	if err != nil {
		t.Fatal(err)
	}
	// end push testing builders to registry

	_, err = docker.GetPlatformImage(nonMultiArchBuilder, "windows/amd64")
	if err == nil {
		t.Error("expected error but got nil")
	}

	_, err = docker.GetPlatformImage(multiArchBuilder, "windows/amd64")
	if err == nil {
		t.Error("expected error but got nil")
	}

	var ref string

	ref, err = docker.GetPlatformImage(nonMultiArchBuilder, "linux/ppc64le")
	if err != nil {
		t.Errorf("unexpeced error: %v", err)
	}
	if ref != nonMultiArchBuilder {
		t.Error("incorrect reference")
	}

	ref, err = docker.GetPlatformImage(multiArchBuilder, "linux/ppc64le")
	if err != nil {
		t.Errorf("unexpeced error: %v", err)
	}

	imgDigest, err := img.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if ref != testRegistry+"/default/builder@"+imgDigest.String() {
		t.Errorf("incorrect reference: %q", ref)
	}
}

func startRegistry(t *testing.T) (addr string) {
	s := http.Server{
		Handler: registry.New(registry.Logger(log.New(io.Discard, "", 0))),
	}
	t.Cleanup(func() { s.Close() })

	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	addr = l.Addr().String()

	go func() {
		err = s.Serve(l)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(os.Stderr, "ERROR: ", err)
		}
	}()

	return addr
}
