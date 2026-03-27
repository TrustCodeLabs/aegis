package aegis_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"aegis"
	"aegis/drivers/localstorage"
)

type storageWriteInput struct {
	Path string `json:"path"`
	Data []byte `json:"data"`
}

type storageReadInput struct {
	Path string `json:"path"`
}

func TestLayeredStorageBindingWritesAllLayersAndReadsThrough(t *testing.T) {
	cacheRoot := t.TempDir()
	baseRoot := t.TempDir()

	registry := aegis.NewDriverRegistry()
	if err := localstorage.Register(registry); err != nil {
		t.Fatalf("register local driver: %v", err)
	}

	module := aegis.NewModule(
		"storage",
		aegis.DefineOperation[storageWriteInput, bool](aegis.OperationSpec[storageWriteInput, bool]{
			Name: "storage.layered.save",
			RequiredCapabilities: []aegis.CapabilityRef{
				"storage.write:user-uploads",
			},
			Handler: func(ctx context.Context, exec aegis.ExecutionContext, input storageWriteInput) (bool, error) {
				err := exec.Resources.Storage("user-uploads").Write(ctx, input.Path, input.Data)
				return err == nil, err
			},
		}),
		aegis.DefineOperation[storageReadInput, []byte](aegis.OperationSpec[storageReadInput, []byte]{
			Name: "storage.layered.read",
			RequiredCapabilities: []aegis.CapabilityRef{
				"storage.read:user-uploads",
			},
			Handler: func(ctx context.Context, exec aegis.ExecutionContext, input storageReadInput) ([]byte, error) {
				return exec.Resources.Storage("user-uploads").Read(ctx, input.Path)
			},
		}),
	)

	kernel, err := aegis.NewBuilder(aegis.Config{
		Resources: aegis.ResourcesConfig{
			Storage: map[string]aegis.StorageBinding{
				"user-uploads": {
					Driver: "layered",
					Layers: []aegis.StorageBinding{
						{
							Driver:   "local",
							Provider: "cache-layer",
							Config:   map[string]any{"root": cacheRoot},
						},
						{
							Driver:   "local",
							Provider: "base-layer",
							Config:   map[string]any{"root": baseRoot},
						},
					},
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

	ctx := aegis.WithCapabilityRefs(context.Background(),
		"storage.write:user-uploads",
		"storage.read:user-uploads",
	)
	_, err = kernel.Execute(ctx, "storage.layered.save", storageWriteInput{
		Path: "docs/hello.txt",
		Data: []byte("world"),
	})
	if err != nil {
		t.Fatalf("execute layered save: %v", err)
	}

	for _, root := range []string{cacheRoot, baseRoot} {
		data, readErr := os.ReadFile(filepath.Join(root, "docs/hello.txt"))
		if readErr != nil {
			t.Fatalf("read from root %s: %v", root, readErr)
		}
		if string(data) != "world" {
			t.Fatalf("unexpected contents in %s: %q", root, string(data))
		}
	}

	if err := os.Remove(filepath.Join(cacheRoot, "docs/hello.txt")); err != nil {
		t.Fatalf("remove cache file: %v", err)
	}

	output, err := kernel.Execute(ctx, "storage.layered.read", storageReadInput{Path: "docs/hello.txt"})
	if err != nil {
		t.Fatalf("execute layered read: %v", err)
	}
	data, ok := output.([]byte)
	if !ok {
		t.Fatalf("unexpected output type: %T", output)
	}
	if string(data) != "world" {
		t.Fatalf("unexpected read data: %q", string(data))
	}

	rewarmed, err := os.ReadFile(filepath.Join(cacheRoot, "docs/hello.txt"))
	if err != nil {
		t.Fatalf("read rewarmed cache file: %v", err)
	}
	if string(rewarmed) != "world" {
		t.Fatalf("unexpected rewarmed contents: %q", string(rewarmed))
	}
}

func TestMultiTenantStorageBindingIsolatesTenants(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()

	registry := aegis.NewDriverRegistry()
	if err := localstorage.Register(registry); err != nil {
		t.Fatalf("register local driver: %v", err)
	}

	module := aegis.NewModule(
		"storage",
		aegis.DefineOperation[storageWriteInput, bool](aegis.OperationSpec[storageWriteInput, bool]{
			Name: "storage.tenant.save",
			RequiredCapabilities: []aegis.CapabilityRef{
				"storage.write:user-uploads",
			},
			Handler: func(ctx context.Context, exec aegis.ExecutionContext, input storageWriteInput) (bool, error) {
				err := exec.Resources.Storage("user-uploads").Write(ctx, input.Path, input.Data)
				return err == nil, err
			},
		}),
	)

	kernel, err := aegis.NewBuilder(aegis.Config{
		Resources: aegis.ResourcesConfig{
			Storage: map[string]aegis.StorageBinding{
				"user-uploads": {
					Tenant: map[string]aegis.StorageBinding{
						"A": {
							Driver: "local",
							Config: map[string]any{"root": rootA},
						},
						"B": {
							Driver: "local",
							Config: map[string]any{"root": rootB},
						},
					},
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

	ctxA := aegis.WithTenantID(
		aegis.WithCapabilityRefs(context.Background(), "storage.write:user-uploads"),
		"A",
	)
	_, err = kernel.Execute(ctxA, "storage.tenant.save", storageWriteInput{
		Path: "tenant-a.txt",
		Data: []byte("A"),
	})
	if err != nil {
		t.Fatalf("execute tenant A save: %v", err)
	}

	ctxB := aegis.WithTenantID(
		aegis.WithCapabilityRefs(context.Background(), "storage.write:user-uploads"),
		"B",
	)
	_, err = kernel.Execute(ctxB, "storage.tenant.save", storageWriteInput{
		Path: "tenant-b.txt",
		Data: []byte("B"),
	})
	if err != nil {
		t.Fatalf("execute tenant B save: %v", err)
	}

	if _, err := os.Stat(filepath.Join(rootA, "tenant-a.txt")); err != nil {
		t.Fatalf("expected tenant A file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootB, "tenant-b.txt")); err != nil {
		t.Fatalf("expected tenant B file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootA, "tenant-b.txt")); !os.IsNotExist(err) {
		t.Fatalf("did not expect tenant B file in root A: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootB, "tenant-a.txt")); !os.IsNotExist(err) {
		t.Fatalf("did not expect tenant A file in root B: %v", err)
	}

	_, err = kernel.Execute(
		aegis.WithCapabilityRefs(context.Background(), "storage.write:user-uploads"),
		"storage.tenant.save",
		storageWriteInput{Path: "missing.txt", Data: []byte("x")},
	)
	if err == nil {
		t.Fatalf("expected tenant binding error")
	}
	if !aegis.IsCode(err, aegis.CodeTenantBindingMissing) {
		t.Fatalf("expected tenant binding missing error, got %v", err)
	}
}

func TestHotSwapStorageBindingRebindsProvider(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()

	registry := aegis.NewDriverRegistry()
	if err := localstorage.Register(registry); err != nil {
		t.Fatalf("register local driver: %v", err)
	}

	module := aegis.NewModule(
		"storage",
		aegis.DefineOperation[storageWriteInput, bool](aegis.OperationSpec[storageWriteInput, bool]{
			Name: "storage.swap.save",
			RequiredCapabilities: []aegis.CapabilityRef{
				"storage.write:user-uploads",
			},
			Effects: []aegis.EffectSpec{
				{
					Name:        "storage.write.user-uploads",
					Kind:        "storage.write",
					RequiredCap: "storage.write:user-uploads",
					Metadata:    map[string]any{"resource": "user-uploads"},
				},
			},
			Handler: func(ctx context.Context, exec aegis.ExecutionContext, input storageWriteInput) (bool, error) {
				err := exec.Resources.Storage("user-uploads").Write(ctx, input.Path, input.Data)
				return err == nil, err
			},
		}),
	)

	kernel, err := aegis.NewBuilder(aegis.Config{
		Resources: aegis.ResourcesConfig{
			Storage: map[string]aegis.StorageBinding{
				"user-uploads": {
					Driver:       "local",
					Config:       map[string]any{"root": rootA},
					HotSwappable: true,
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

	ctx := aegis.WithCapabilityRefs(context.Background(), "storage.write:user-uploads")
	_, err = kernel.Execute(ctx, "storage.swap.save", storageWriteInput{
		Path: "one.txt",
		Data: []byte("one"),
	})
	if err != nil {
		t.Fatalf("execute pre-swap save: %v", err)
	}

	if err := kernel.SwapStorageBinding(context.Background(), "user-uploads", aegis.StorageBinding{
		Driver:       "local",
		Config:       map[string]any{"root": rootB},
		HotSwappable: true,
	}); err != nil {
		t.Fatalf("swap storage binding: %v", err)
	}

	_, err = kernel.Execute(ctx, "storage.swap.save", storageWriteInput{
		Path: "two.txt",
		Data: []byte("two"),
	})
	if err != nil {
		t.Fatalf("execute post-swap save: %v", err)
	}

	if _, err := os.Stat(filepath.Join(rootA, "one.txt")); err != nil {
		t.Fatalf("expected pre-swap file in root A: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootB, "two.txt")); err != nil {
		t.Fatalf("expected post-swap file in root B: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootA, "two.txt")); !os.IsNotExist(err) {
		t.Fatalf("did not expect post-swap file in root A: %v", err)
	}
}
