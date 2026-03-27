package aegis

import (
	"errors"
	"fmt"
)

const (
	CodeAuditFailed            = "audit_failed"
	CodeBootstrapFailed        = "bootstrap_failed"
	CodeCapabilityDenied       = "capability_denied"
	CodeConfirmationNeeded     = "confirmation_required"
	CodeDriverNotRegistered    = "driver_not_registered"
	CodeDriverUnhealthy        = "driver_unhealthy"
	CodeDuplicateOperation     = "duplicate_operation"
	CodeDuplicateModule        = "duplicate_module"
	CodeDuplicatePolicy        = "duplicate_policy"
	CodeDuplicateResource      = "duplicate_resource"
	CodeEffectDenied           = "effect_denied"
	CodeEffectViolation        = "effect_violation"
	CodeHotSwapDenied          = "hot_swap_denied"
	CodeInvalidConfig          = "invalid_config"
	CodeInvalidInput           = "invalid_input"
	CodeResourceNotImplemented = "resource_not_implemented"
	CodeOperationNotFound      = "operation_not_found"
	CodePolicyDenied           = "policy_denied"
	CodePolicyNotFound         = "policy_not_found"
	CodeResourceNotFound       = "resource_not_found"
	CodeTenantBindingMissing   = "tenant_binding_missing"
)

type KernelError struct {
	Code    string
	Message string
	Cause   error
}

func (e *KernelError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
}

func (e *KernelError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func IsCode(err error, code string) bool {
	var kernelErr *KernelError
	if !errors.As(err, &kernelErr) {
		return false
	}
	return kernelErr.Code == code
}

func NewKernelError(code, message string, cause error) error {
	return newKernelError(code, message, cause)
}

func newKernelError(code, message string, cause error) error {
	return &KernelError{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}
