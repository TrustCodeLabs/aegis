package localstorage_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"aegis"
	"aegis/drivers/localstorage"
)

func TestDriverRejectsEscapingPaths(t *testing.T) {
	driver, err := localstorage.New(t.TempDir())
	if err != nil {
		t.Fatalf("create driver: %v", err)
	}

	err = driver.Write(context.Background(), "../secret.txt", []byte("nope"))
	if err == nil {
		t.Fatalf("expected path escape to fail")
	}
	if !strings.Contains(err.Error(), "escapes storage root") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDriverReadDeleteHealthAndRegistration(t *testing.T) {
	root := t.TempDir()

	driver, err := localstorage.New(root)
	if err != nil {
		t.Fatalf("create driver: %v", err)
	}

	if err := driver.Write(context.Background(), "docs/file.txt", []byte("hello")); err != nil {
		t.Fatalf("write file: %v", err)
	}

	data, err := driver.Read(context.Background(), "docs/file.txt")
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected file contents: %q", string(data))
	}

	if err := driver.Delete(context.Background(), "docs/file.txt"); err != nil {
		t.Fatalf("delete file: %v", err)
	}
	if _, err := os.Stat(root + "/docs/file.txt"); !os.IsNotExist(err) {
		t.Fatalf("expected file to be deleted, got %v", err)
	}

	if err := driver.Delete(context.Background(), "docs/file.txt"); err != nil {
		t.Fatalf("delete missing file should be ignored, got %v", err)
	}

	if err := driver.HealthCheck(context.Background()); err != nil {
		t.Fatalf("health check: %v", err)
	}

	registry := aegis.NewDriverRegistry()
	if err := localstorage.Register(registry); err != nil {
		t.Fatalf("register driver: %v", err)
	}
	if err := localstorage.Register(registry); err == nil {
		t.Fatalf("expected duplicate registration to fail")
	}

	factoryDriver := localstorage.Factory()
	if factoryDriver == nil {
		t.Fatalf("expected factory to return a driver")
	}
}

func TestDriverInitAndReadFailures(t *testing.T) {
	driver := localstorage.Factory()
	if err := driver.Init(map[string]any{}); err == nil {
		t.Fatalf("expected init without root to fail")
	}

	validDriver, err := localstorage.New(t.TempDir())
	if err != nil {
		t.Fatalf("create driver: %v", err)
	}

	_, err = validDriver.Read(context.Background(), "missing.txt")
	if err == nil {
		t.Fatalf("expected missing read to fail")
	}
	if !aegis.IsNotFoundError(err) {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestDriverEdgeCases(t *testing.T) {
	if err := localstorage.Register(nil); err == nil {
		t.Fatalf("expected nil registry registration to fail")
	}

	driver, err := localstorage.New(t.TempDir())
	if err != nil {
		t.Fatalf("create driver: %v", err)
	}

	if err := driver.Write(context.Background(), "/absolute.txt", []byte("bad")); err == nil {
		t.Fatalf("expected absolute paths to be rejected")
	}
	if _, err := driver.Read(context.Background(), ""); err == nil {
		t.Fatalf("expected empty read path to be rejected")
	}

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := driver.Write(cancelledCtx, "docs/file.txt", []byte("x")); err == nil {
		t.Fatalf("expected cancelled write to fail")
	}
	if _, err := driver.Read(cancelledCtx, "docs/file.txt"); err == nil {
		t.Fatalf("expected cancelled read to fail")
	}
	if err := driver.Delete(cancelledCtx, "docs/file.txt"); err == nil {
		t.Fatalf("expected cancelled delete to fail")
	}
	if err := driver.HealthCheck(cancelledCtx); err == nil {
		t.Fatalf("expected cancelled health check to fail")
	}

	uninitialized := localstorage.Factory()
	if err := uninitialized.HealthCheck(context.Background()); err == nil {
		t.Fatalf("expected health check on uninitialized driver to fail")
	}
}
