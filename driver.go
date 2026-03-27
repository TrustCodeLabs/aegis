package aegis

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"time"
)

const (
	DriverErrorKindInvalidInput = "invalid_input"
	DriverErrorKindNotFound     = "not_found"
	DriverErrorKindUnavailable  = "unavailable"
	DriverErrorKindInternal     = "internal"
)

type DriverError struct {
	Provider  string
	Driver    string
	Operation string
	Kind      string
	Retryable bool
	Cause     error
}

func (e *DriverError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause == nil {
		return fmt.Sprintf("driver %s/%s %s failed", e.Driver, e.Provider, e.Operation)
	}
	return fmt.Sprintf("driver %s/%s %s failed: %v", e.Driver, e.Provider, e.Operation, e.Cause)
}

func (e *DriverError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func IsDriverKind(err error, kind string) bool {
	var driverErr *DriverError
	if !errors.As(err, &driverErr) {
		return false
	}
	return driverErr.Kind == kind
}

func IsNotFoundError(err error) bool {
	return errors.Is(err, os.ErrNotExist) || IsDriverKind(err, DriverErrorKindNotFound)
}

type DriverStats struct {
	OperationCount uint64
	ErrorCount     uint64
	LastLatency    time.Duration
}

type StorageDriver interface {
	Init(config map[string]any) error
	Write(ctx context.Context, path string, data []byte) error
	Read(ctx context.Context, path string) ([]byte, error)
	Delete(ctx context.Context, path string) error
	HealthCheck(ctx context.Context) error
}

type StorageDriverFactory func() StorageDriver

type observedStorageDriver struct {
	provider string
	driver   string
	inner    StorageDriver
	stats    storageDriverStats
}

type storageDriverStats struct {
	operationCount atomic.Uint64
	errorCount     atomic.Uint64
	lastLatencyNS  atomic.Int64
}

func newObservedStorageDriver(providerName, driverName string, driver StorageDriver) *observedStorageDriver {
	return &observedStorageDriver{
		provider: providerName,
		driver:   driverName,
		inner:    driver,
	}
}

func (d *observedStorageDriver) Stats() DriverStats {
	return DriverStats{
		OperationCount: d.stats.operationCount.Load(),
		ErrorCount:     d.stats.errorCount.Load(),
		LastLatency:    time.Duration(d.stats.lastLatencyNS.Load()),
	}
}

func (d *observedStorageDriver) HealthCheck(ctx context.Context) error {
	return d.inner.HealthCheck(ctx)
}

func (d *observedStorageDriver) Write(ctx context.Context, path string, data []byte) error {
	return d.measure("write", func() error {
		return d.inner.Write(ctx, path, data)
	})
}

func (d *observedStorageDriver) Read(ctx context.Context, path string) ([]byte, error) {
	started := time.Now()
	data, err := d.inner.Read(ctx, path)
	d.stats.operationCount.Add(1)
	d.stats.lastLatencyNS.Store(time.Since(started).Nanoseconds())
	if err != nil {
		d.stats.errorCount.Add(1)
		return nil, wrapDriverError(d.provider, d.driver, "read", err)
	}
	return data, nil
}

func (d *observedStorageDriver) Delete(ctx context.Context, path string) error {
	return d.measure("delete", func() error {
		return d.inner.Delete(ctx, path)
	})
}

func (d *observedStorageDriver) measure(operation string, fn func() error) error {
	started := time.Now()
	err := fn()
	d.stats.operationCount.Add(1)
	d.stats.lastLatencyNS.Store(time.Since(started).Nanoseconds())
	if err != nil {
		d.stats.errorCount.Add(1)
		return wrapDriverError(d.provider, d.driver, operation, err)
	}
	return nil
}

func wrapDriverError(provider, driver, operation string, err error) error {
	var driverErr *DriverError
	if errors.As(err, &driverErr) {
		if driverErr.Provider == "" {
			driverErr.Provider = provider
		}
		if driverErr.Driver == "" {
			driverErr.Driver = driver
		}
		if driverErr.Operation == "" {
			driverErr.Operation = operation
		}
		return driverErr
	}

	kind := DriverErrorKindInternal
	retryable := false
	switch {
	case errors.Is(err, os.ErrNotExist):
		kind = DriverErrorKindNotFound
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		kind = DriverErrorKindUnavailable
		retryable = true
	}

	return &DriverError{
		Provider:  provider,
		Driver:    driver,
		Operation: operation,
		Kind:      kind,
		Retryable: retryable,
		Cause:     err,
	}
}
