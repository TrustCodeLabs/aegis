package aegis_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"aegis"
	"aegis/drivers/localstorage"
)

type writeInput struct {
	Path string
	Data []byte
}

type writeOutput struct {
	OK bool
}

func TestKernelExecuteWritesThroughResourceLayer(t *testing.T) {
	tmpDir := t.TempDir()

	registry := aegis.NewDriverRegistry()
	if err := localstorage.Register(registry); err != nil {
		t.Fatalf("register local driver: %v", err)
	}

	module := aegis.NewModule(
		"uploads",
		aegis.DefineOperation[writeInput, writeOutput](aegis.OperationSpec[writeInput, writeOutput]{
			Name: "upload.write",
			RequiredCapabilities: []aegis.CapabilityRef{
				"storage.write:user-uploads",
			},
			Validate: func(input writeInput) error {
				if input.Path == "" {
					return errors.New("path is required")
				}
				return nil
			},
			Handler: func(ctx context.Context, exec aegis.ExecutionContext, input writeInput) (writeOutput, error) {
				err := exec.Resources.Storage("user-uploads").Write(ctx, input.Path, input.Data)
				if err != nil {
					return writeOutput{}, err
				}
				return writeOutput{OK: true}, nil
			},
		}),
	)

	kernel, err := aegis.NewBuilder(aegis.Config{
		Resources: aegis.ResourcesConfig{
			Storage: map[string]aegis.StorageBinding{
				"user-uploads": {
					Driver: "local",
					Config: map[string]any{"root": tmpDir},
				},
			},
		},
	}).
		WithDriverRegistry(registry).
		WithModule(module).
		Build()
	if err != nil {
		t.Fatalf("build kernel: %v", err)
	}

	ctx := aegis.WithSubject(context.Background(), aegis.Subject{
		ID:   "user-1",
		Type: "user",
	})
	ctx = aegis.WithCapabilityRefs(ctx, "storage.write:user-uploads")

	rawOutput, err := kernel.Execute(ctx, "upload.write", writeInput{
		Path: "docs/hello.txt",
		Data: []byte("world"),
	})
	if err != nil {
		t.Fatalf("execute kernel: %v", err)
	}

	output, ok := rawOutput.(writeOutput)
	if !ok {
		t.Fatalf("unexpected output type: %T", rawOutput)
	}
	if !output.OK {
		t.Fatalf("expected operation output OK=true")
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "docs/hello.txt"))
	if err != nil {
		t.Fatalf("read stored file: %v", err)
	}
	if string(data) != "world" {
		t.Fatalf("unexpected file contents: %q", string(data))
	}
}

func TestKernelExecuteDeniesMissingResourceCapability(t *testing.T) {
	tmpDir := t.TempDir()

	registry := aegis.NewDriverRegistry()
	if err := localstorage.Register(registry); err != nil {
		t.Fatalf("register local driver: %v", err)
	}

	module := aegis.NewModule(
		"uploads",
		aegis.DefineOperation[writeInput, writeOutput](aegis.OperationSpec[writeInput, writeOutput]{
			Name: "upload.write",
			Handler: func(ctx context.Context, exec aegis.ExecutionContext, input writeInput) (writeOutput, error) {
				err := exec.Resources.Storage("user-uploads").Write(ctx, input.Path, input.Data)
				if err != nil {
					return writeOutput{}, err
				}
				return writeOutput{OK: true}, nil
			},
		}),
	)

	kernel, err := aegis.NewBuilder(aegis.Config{
		Resources: aegis.ResourcesConfig{
			Storage: map[string]aegis.StorageBinding{
				"user-uploads": {
					Driver: "local",
					Config: map[string]any{"root": tmpDir},
				},
			},
		},
	}).
		WithDriverRegistry(registry).
		WithModule(module).
		Build()
	if err != nil {
		t.Fatalf("build kernel: %v", err)
	}

	_, err = kernel.Execute(context.Background(), "upload.write", writeInput{
		Path: "docs/hello.txt",
		Data: []byte("world"),
	})
	if err == nil {
		t.Fatalf("expected capability denial")
	}
	if !aegis.IsCode(err, aegis.CodeCapabilityDenied) {
		t.Fatalf("expected capability denial, got: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(tmpDir, "docs/hello.txt")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected file not to be written, stat error: %v", statErr)
	}
}

func TestBuilderFailsForUnknownStorageDriver(t *testing.T) {
	_, err := aegis.NewBuilder(aegis.Config{
		Resources: aegis.ResourcesConfig{
			Storage: map[string]aegis.StorageBinding{
				"user-uploads": {
					Driver: "s3",
				},
			},
		},
	}).Build()
	if err == nil {
		t.Fatalf("expected build to fail")
	}
	if !aegis.IsCode(err, aegis.CodeDriverNotRegistered) {
		t.Fatalf("expected driver not registered, got: %v", err)
	}
}
