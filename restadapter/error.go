package restadapter

import (
	"net/http"

	"aegis"
)

func ClassifyError(err error) (int, string) {
	switch {
	case aegis.IsCode(err, aegis.CodeInvalidInput), aegis.IsDriverKind(err, aegis.DriverErrorKindInvalidInput):
		return http.StatusBadRequest, "invalid_input"
	case aegis.IsCode(err, aegis.CodeCapabilityDenied), aegis.IsCode(err, aegis.CodePolicyDenied):
		return http.StatusForbidden, "forbidden"
	case aegis.IsCode(err, aegis.CodeConfirmationNeeded):
		return http.StatusConflict, "confirmation_required"
	case aegis.IsCode(err, aegis.CodeResourceNotFound),
		aegis.IsCode(err, aegis.CodeOperationNotFound),
		aegis.IsCode(err, aegis.CodeTenantBindingMissing),
		aegis.IsNotFoundError(err):
		return http.StatusNotFound, "not_found"
	case aegis.IsCode(err, aegis.CodeResourceNotImplemented):
		return http.StatusNotImplemented, "not_implemented"
	case aegis.IsCode(err, aegis.CodeHotSwapDenied), aegis.IsCode(err, aegis.CodeEffectViolation):
		return http.StatusConflict, "conflict"
	default:
		return http.StatusInternalServerError, "internal_error"
	}
}

func DefaultErrorEncoder(w http.ResponseWriter, r *http.Request, err error) {
	status, code := ClassifyError(err)
	_ = WriteJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": err.Error(),
		},
	})
}
