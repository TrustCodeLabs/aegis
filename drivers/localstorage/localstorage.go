package localstorage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aegis"
)

type Driver struct {
	root string
}

func Register(registry *aegis.DriverRegistry) error {
	if registry == nil {
		return fmt.Errorf("driver registry cannot be nil")
	}
	return registry.RegisterStorage("local", Factory)
}

func Factory() aegis.StorageDriver {
	return &Driver{}
}

func New(root string) (*Driver, error) {
	driver := &Driver{}
	if err := driver.Init(map[string]any{"root": root}); err != nil {
		return nil, err
	}
	return driver, nil
}

func (d *Driver) Init(config map[string]any) error {
	rawRoot, _ := config["root"]
	root, _ := rawRoot.(string)
	root = strings.TrimSpace(root)
	if root == "" {
		return &aegis.DriverError{
			Driver:    "local",
			Operation: "init",
			Kind:      aegis.DriverErrorKindInvalidInput,
			Cause:     fmt.Errorf("local storage driver requires config.root"),
		}
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return &aegis.DriverError{
			Driver:    "local",
			Operation: "init",
			Kind:      aegis.DriverErrorKindInternal,
			Cause:     fmt.Errorf("resolve storage root: %w", err),
		}
	}

	d.root = filepath.Clean(absRoot)
	if err := os.MkdirAll(d.root, 0o755); err != nil {
		return &aegis.DriverError{
			Driver:    "local",
			Operation: "init",
			Kind:      aegis.DriverErrorKindUnavailable,
			Retryable: true,
			Cause:     err,
		}
	}
	return nil
}

func (d *Driver) Write(ctx context.Context, path string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	target, err := d.resolve(path)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	return os.WriteFile(target, data, 0o644)
}

func (d *Driver) Read(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	target, err := d.resolve(path)
	if err != nil {
		return nil, err
	}

	return os.ReadFile(target)
}

func (d *Driver) Delete(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	target, err := d.resolve(path)
	if err != nil {
		return err
	}

	err = os.Remove(target)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (d *Driver) HealthCheck(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d.root == "" {
		return &aegis.DriverError{
			Driver:    "local",
			Operation: "health_check",
			Kind:      aegis.DriverErrorKindInvalidInput,
			Cause:     fmt.Errorf("driver is not initialized"),
		}
	}
	_, err := os.Stat(d.root)
	if err != nil {
		return &aegis.DriverError{
			Driver:    "local",
			Operation: "health_check",
			Kind:      aegis.DriverErrorKindUnavailable,
			Retryable: true,
			Cause:     err,
		}
	}
	return nil
}

func (d *Driver) resolve(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path cannot be empty")
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("absolute paths are not allowed: %q", path)
	}

	target := filepath.Join(d.root, filepath.Clean(path))
	rel, err := filepath.Rel(d.root, target)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, fmt.Sprintf("..%c", os.PathSeparator)) {
		return "", fmt.Errorf("path %q escapes storage root", path)
	}

	return target, nil
}
