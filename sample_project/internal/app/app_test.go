package app

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
)

func TestAppLifecycleAndConfigHelpers(t *testing.T) {
	tempDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	logger := log.New(io.Discard, "", 0)
	application, err := New(logger)
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	if application.Kernel() == nil {
		t.Fatalf("expected kernel to be initialized")
	}
	if application.Logger() == nil {
		t.Fatalf("expected logger to be initialized")
	}
	if application.StorageMode() != StorageModeLayered {
		t.Fatalf("expected initial mode %q, got %q", StorageModeLayered, application.StorageMode())
	}

	if err := application.GenerateSkillsMarkdown(); err != nil {
		t.Fatalf("generate skills markdown: %v", err)
	}
	if _, err := os.Stat(filepath.Clean(SkillsOutputPath)); err != nil {
		t.Fatalf("expected generated skills file: %v", err)
	}

	ops, err := application.SwapStorageMode(context.Background(), StorageModeDirect)
	if err != nil {
		t.Fatalf("swap storage mode: %v", err)
	}
	if len(ops) == 0 {
		t.Fatalf("expected operations after swap")
	}
	if application.StorageMode() != StorageModeDirect {
		t.Fatalf("expected direct mode after swap, got %q", application.StorageMode())
	}

	if _, err := application.SwapStorageMode(context.Background(), "unsupported"); err == nil {
		t.Fatalf("expected invalid mode to fail")
	}

	t.Setenv("AEGIS_SAMPLE_ENV", "configured")
	if got := EnvOrDefault("AEGIS_SAMPLE_ENV", "fallback"); got != "configured" {
		t.Fatalf("unexpected env override: %q", got)
	}
	if got := EnvOrDefault("AEGIS_SAMPLE_MISSING", "fallback"); got != "fallback" {
		t.Fatalf("unexpected env fallback: %q", got)
	}
	if NormalizeStorageMode("  DIRECT ") != StorageModeDirect {
		t.Fatalf("expected normalized direct mode")
	}
	if !IsValidStorageMode(StorageModeLayered) || !IsValidStorageMode(StorageModeDirect) || IsValidStorageMode("invalid") {
		t.Fatalf("unexpected storage mode validation result")
	}
}
