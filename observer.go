package aegis

import "context"

type ObservabilityHooks interface {
	OperationStarted(ctx context.Context, exec ExecutionContext)
	OperationFinished(ctx context.Context, report ExecutionReport)
	CapabilityChecked(exec ExecutionContext, capability CapabilityRef, allowed bool)
}

type NoopObserver struct{}

func (NoopObserver) OperationStarted(ctx context.Context, exec ExecutionContext) {}

func (NoopObserver) OperationFinished(ctx context.Context, report ExecutionReport) {}

func (NoopObserver) CapabilityChecked(exec ExecutionContext, capability CapabilityRef, allowed bool) {
}
