package mcp

import (
	"context"
	"testing"

	fn "knative.dev/func/pkg/functions"
)

func testService(t *testing.T) *Service {
	t.Helper()
	return NewService(testClientFactory)
}

func TestService_ConfigEnvsAdd(t *testing.T) {
	path := initTestFunction(t)
	svc := testService(t)

	name := "API_KEY"
	value := "secret123"
	out, err := svc.ConfigEnvsAdd(context.Background(), ConfigEnvsAddInput{
		Path:  path,
		Name:  &name,
		Value: &value,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Message == "" {
		t.Fatal("expected message")
	}

	f, err := fn.NewFunction(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Run.Envs) != 1 {
		t.Fatalf("expected 1 env, got %d", len(f.Run.Envs))
	}
}

func TestService_ConfigEnvsAdd_SecretKey(t *testing.T) {
	path := initTestFunction(t)
	svc := testService(t)

	name := "DB_PASS"
	secret := "my-secret"
	key := "password"
	out, err := svc.ConfigEnvsAdd(context.Background(), ConfigEnvsAddInput{
		Path:       path,
		Name:       &name,
		SecretName: &secret,
		SecretKey:  &key,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Message == "" {
		t.Fatal("expected message")
	}
}

func TestService_ConfigLabelsAdd(t *testing.T) {
	path := initTestFunction(t)
	svc := testService(t)

	name := "app"
	value := "demo"
	_, err := svc.ConfigLabelsAdd(context.Background(), ConfigLabelsAddInput{
		Path:  path,
		Name:  &name,
		Value: &value,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestService_ConfigVolumesAdd(t *testing.T) {
	path := initTestFunction(t)
	svc := testService(t)

	volType := "emptydir"
	mountPath := "/tmp/cache"
	_, err := svc.ConfigVolumesAdd(context.Background(), ConfigVolumesAddInput{
		Path:      path,
		Type:      &volType,
		MountPath: &mountPath,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestService_List(t *testing.T) {
	svc := testService(t)
	out, err := svc.List(context.Background(), ListInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Message == "" {
		t.Fatal("expected message")
	}
}

func TestService_Create(t *testing.T) {
	svc := testService(t)
	dir := t.TempDir()
	out, err := svc.Create(context.Background(), CreateInput{
		Language: "go",
		Path:     dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Runtime != "go" {
		t.Fatalf("expected runtime go, got %s", out.Runtime)
	}
}
