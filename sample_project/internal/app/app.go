package app

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"

	"aegis"
	"aegis/sample_project/internal/notes"
)

type App struct {
	kernel *aegis.Kernel
	logger *log.Logger

	modeMu sync.RWMutex
	mode   string
}

func New(logger *log.Logger) (*App, error) {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}

	kernel, err := buildKernel(StorageModeLayered)
	if err != nil {
		return nil, err
	}

	return &App{
		kernel: kernel,
		logger: logger,
		mode:   StorageModeLayered,
	}, nil
}

func (a *App) Kernel() *aegis.Kernel {
	return a.kernel
}

func (a *App) Logger() *log.Logger {
	return a.logger
}

func (a *App) StorageMode() string {
	a.modeMu.RLock()
	defer a.modeMu.RUnlock()
	return a.mode
}

func (a *App) GenerateSkillsMarkdown() error {
	return a.kernel.WriteSkillsMarkdown(context.Background(), aegis.IntrospectionFilter{
		VisibilityTier: VisibilityTierInternal,
	}, SkillsOutputPath)
}

func (a *App) SwapStorageMode(ctx context.Context, mode string) ([]aegis.OperationInfo, error) {
	mode = NormalizeStorageMode(mode)
	if !IsValidStorageMode(mode) {
		return nil, fmt.Errorf("unsupported storage mode %q", mode)
	}

	if err := a.kernel.SwapStorageBinding(ctx, notes.ResourceName, storageBindingForMode(mode)); err != nil {
		return nil, err
	}

	a.modeMu.Lock()
	a.mode = mode
	a.modeMu.Unlock()

	if err := a.GenerateSkillsMarkdown(); err != nil {
		a.logger.Printf("failed to refresh generated skills after swap: %v", err)
	}

	return a.kernel.Operations(ctx, aegis.IntrospectionFilter{
		Module:         notes.ModuleName,
		VisibilityTier: VisibilityTierInternal,
	})
}
