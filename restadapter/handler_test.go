package restadapter_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aegis"
	"aegis/restadapter"
)

type echoInput struct {
	Message string `json:"message"`
}

func TestNewJSONHandlerExecutesKernelOperation(t *testing.T) {
	module := aegis.NewModule(
		"echo",
		aegis.DefineOperation[echoInput, map[string]any](aegis.OperationSpec[echoInput, map[string]any]{
			Name: "echo.reply",
			Handler: func(ctx context.Context, exec aegis.ExecutionContext, input echoInput) (map[string]any, error) {
				return map[string]any{
					"message":    input.Message,
					"request_id": exec.RequestID,
				}, nil
			},
		}),
	)

	kernel, err := aegis.NewBuilder(aegis.Config{}).
		WithModule(module).
		Build()
	if err != nil {
		t.Fatalf("build kernel: %v", err)
	}

	handler := restadapter.NewJSONHandler(restadapter.Operation[echoInput]{
		Kernel:    kernel,
		Operation: "echo.reply",
		ContextBuilder: func(ctx context.Context, r *http.Request) (context.Context, error) {
			return aegis.WithRequestID(ctx, "req-rest-1"), nil
		},
		InputBinder:   restadapter.DecodeJSONBody[echoInput],
		SuccessStatus: http.StatusCreated,
	})

	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(`{"message":"hello"}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"message": "hello"`) {
		t.Fatalf("expected JSON body to include message, got %s", body)
	}
	if !strings.Contains(body, `"request_id": "req-rest-1"`) {
		t.Fatalf("expected JSON body to include request id, got %s", body)
	}
}

func TestNewJSONEndpointCoversSuccessBindAndHandlerErrors(t *testing.T) {
	success := restadapter.NewJSONEndpoint(restadapter.Endpoint[echoInput]{
		ContextBuilder: func(ctx context.Context, r *http.Request) (context.Context, error) {
			return aegis.WithRequestID(ctx, "req-endpoint-1"), nil
		},
		InputBinder: restadapter.DecodeJSONBody[echoInput],
		Handler: func(ctx context.Context, input echoInput) (any, error) {
			return map[string]any{
				"message":    input.Message,
				"request_id": aegis.RequestIDFromContext(ctx),
			}, nil
		},
		SuccessStatus: http.StatusAccepted,
	})

	successRec := httptest.NewRecorder()
	success.ServeHTTP(successRec, httptest.NewRequest(http.MethodPost, "/endpoint", strings.NewReader(`{"message":"ok"}`)))
	if successRec.Code != http.StatusAccepted {
		t.Fatalf("expected endpoint success to return 202, got %d", successRec.Code)
	}
	if !strings.Contains(successRec.Body.String(), `"request_id": "req-endpoint-1"`) {
		t.Fatalf("unexpected endpoint success body: %s", successRec.Body.String())
	}

	bindErr := restadapter.NewJSONEndpoint(restadapter.Endpoint[echoInput]{
		InputBinder: restadapter.DecodeJSONBody[echoInput],
		Handler: func(ctx context.Context, input echoInput) (any, error) {
			return map[string]any{"ok": true}, nil
		},
	})
	bindRec := httptest.NewRecorder()
	bindErr.ServeHTTP(bindRec, httptest.NewRequest(http.MethodPost, "/endpoint", strings.NewReader(`{"message":`)))
	if bindRec.Code != http.StatusBadRequest {
		t.Fatalf("expected bind error to return 400, got %d", bindRec.Code)
	}

	handlerErr := restadapter.NewJSONEndpoint(restadapter.Endpoint[struct{}]{
		Handler: func(ctx context.Context, input struct{}) (any, error) {
			return nil, aegis.NewKernelError(aegis.CodePolicyDenied, "denied", nil)
		},
	})
	handlerRec := httptest.NewRecorder()
	handlerErr.ServeHTTP(handlerRec, httptest.NewRequest(http.MethodGet, "/endpoint", nil))
	if handlerRec.Code != http.StatusForbidden {
		t.Fatalf("expected handler error to return 403, got %d", handlerRec.Code)
	}
}

func TestDecodeJSONBodyReturnsInvalidInputForMalformedJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(`{"message":`))

	_, err := restadapter.DecodeJSONBody[echoInput](req)
	if err == nil {
		t.Fatalf("expected invalid json error")
	}
	if !aegis.IsCode(err, aegis.CodeInvalidInput) {
		t.Fatalf("expected invalid input error, got %v", err)
	}
}

func TestHelpersAndErrorEncoders(t *testing.T) {
	noInput, err := restadapter.NoInput[echoInput]()(httptest.NewRequest(http.MethodGet, "/echo", nil))
	if err != nil {
		t.Fatalf("no input binder: %v", err)
	}
	if noInput.Message != "" {
		t.Fatalf("expected zero-value input, got %#v", noInput)
	}

	staticInput, err := restadapter.StaticInput(echoInput{Message: "static"})(httptest.NewRequest(http.MethodGet, "/echo", nil))
	if err != nil {
		t.Fatalf("static input binder: %v", err)
	}
	if staticInput.Message != "static" {
		t.Fatalf("unexpected static input: %#v", staticInput)
	}

	status, code := restadapter.ClassifyError(aegis.NewKernelError(aegis.CodeResourceNotImplemented, "not ready", nil))
	if status != http.StatusNotImplemented || code != "not_implemented" {
		t.Fatalf("unexpected classified error: status=%d code=%q", status, code)
	}

	rec := httptest.NewRecorder()
	restadapter.DefaultErrorEncoder(rec, httptest.NewRequest(http.MethodGet, "/echo", nil), aegis.NewKernelError(aegis.CodeInvalidInput, "bad request", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"code": "invalid_input"`) {
		t.Fatalf("unexpected default error body: %s", rec.Body.String())
	}

	cases := []struct {
		err        error
		wantStatus int
		wantCode   string
	}{
		{err: aegis.NewKernelError(aegis.CodeCapabilityDenied, "forbidden", nil), wantStatus: http.StatusForbidden, wantCode: "forbidden"},
		{err: aegis.NewKernelError(aegis.CodeConfirmationNeeded, "confirm", nil), wantStatus: http.StatusConflict, wantCode: "confirmation_required"},
		{err: aegis.NewKernelError(aegis.CodeResourceNotFound, "missing", nil), wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{err: &aegis.DriverError{Kind: aegis.DriverErrorKindInvalidInput}, wantStatus: http.StatusBadRequest, wantCode: "invalid_input"},
		{err: errors.New("boom"), wantStatus: http.StatusInternalServerError, wantCode: "internal_error"},
	}

	for _, tc := range cases {
		status, code := restadapter.ClassifyError(tc.err)
		if status != tc.wantStatus || code != tc.wantCode {
			t.Fatalf("unexpected error classification for %v: status=%d code=%q", tc.err, status, code)
		}
	}
}

func TestNewJSONHandlerCoversErrorBranches(t *testing.T) {
	kernelHandler := restadapter.NewJSONHandler(restadapter.Operation[echoInput]{
		Operation: "missing.kernel",
	})
	kernelRec := httptest.NewRecorder()
	kernelHandler.ServeHTTP(kernelRec, httptest.NewRequest(http.MethodGet, "/echo", nil))
	if kernelRec.Code != http.StatusInternalServerError {
		t.Fatalf("expected bootstrap failure to return 500, got %d", kernelRec.Code)
	}

	contextErrHandler := restadapter.NewJSONHandler(restadapter.Operation[echoInput]{
		Kernel: aegisMustBuildKernel(t),
		ContextBuilder: func(ctx context.Context, r *http.Request) (context.Context, error) {
			return nil, aegis.NewKernelError(aegis.CodeInvalidInput, "bad context", nil)
		},
	})
	contextRec := httptest.NewRecorder()
	contextErrHandler.ServeHTTP(contextRec, httptest.NewRequest(http.MethodGet, "/echo", nil))
	if contextRec.Code != http.StatusBadRequest {
		t.Fatalf("expected context builder error to return 400, got %d", contextRec.Code)
	}

	kernelErrHandler := restadapter.NewJSONHandler(restadapter.Operation[echoInput]{
		Kernel:    aegisMustBuildKernel(t),
		Operation: "echo.missing",
	})
	kernelErrRec := httptest.NewRecorder()
	kernelErrHandler.ServeHTTP(kernelErrRec, httptest.NewRequest(http.MethodGet, "/echo", nil))
	if kernelErrRec.Code != http.StatusNotFound {
		t.Fatalf("expected missing operation to return 404, got %d", kernelErrRec.Code)
	}

	var captured error
	responseErrHandler := restadapter.NewJSONHandler(restadapter.Operation[struct{}]{
		Kernel:    aegisMustBuildNoInputKernel(t),
		Operation: "echo.no_input",
		ContextBuilder: func(ctx context.Context, r *http.Request) (context.Context, error) {
			return nil, nil
		},
		ResponseEncoder: func(w http.ResponseWriter, status int, payload any) error {
			return io.ErrClosedPipe
		},
		ErrorEncoder: func(w http.ResponseWriter, r *http.Request, err error) {
			captured = err
			w.WriteHeader(http.StatusTeapot)
		},
	})
	responseRec := httptest.NewRecorder()
	responseErrHandler.ServeHTTP(responseRec, httptest.NewRequest(http.MethodGet, "/echo", nil))
	if responseRec.Code != http.StatusTeapot {
		t.Fatalf("expected custom error encoder to be used, got %d", responseRec.Code)
	}
	if !errors.Is(captured, io.ErrClosedPipe) {
		t.Fatalf("expected response encoder error to be forwarded, got %v", captured)
	}
}

func TestDecodeJSONBodyEdgeCases(t *testing.T) {
	nilBodyReq := httptest.NewRequest(http.MethodPost, "/echo", nil)
	nilBodyReq.Body = nil
	if _, err := restadapter.DecodeJSONBody[echoInput](nilBodyReq); !aegis.IsCode(err, aegis.CodeInvalidInput) {
		t.Fatalf("expected nil body to be invalid input, got %v", err)
	}

	emptyReq := httptest.NewRequest(http.MethodPost, "/echo", http.NoBody)
	if _, err := restadapter.DecodeJSONBody[echoInput](emptyReq); !aegis.IsCode(err, aegis.CodeInvalidInput) {
		t.Fatalf("expected empty body to be invalid input, got %v", err)
	}

	multiReq := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(`{"message":"one"}{"message":"two"}`))
	if _, err := restadapter.DecodeJSONBody[echoInput](multiReq); !aegis.IsCode(err, aegis.CodeInvalidInput) {
		t.Fatalf("expected multiple JSON values to be invalid, got %v", err)
	}

	unknownFieldReq := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(`{"message":"ok","extra":true}`))
	if _, err := restadapter.DecodeJSONBody[echoInput](unknownFieldReq); !aegis.IsCode(err, aegis.CodeInvalidInput) {
		t.Fatalf("expected unknown field payload to be invalid, got %v", err)
	}
}

func aegisMustBuildKernel(t *testing.T) *aegis.Kernel {
	t.Helper()

	module := aegis.NewModule(
		"echo",
		aegis.DefineOperation[echoInput, map[string]any](aegis.OperationSpec[echoInput, map[string]any]{
			Name: "echo.reply",
			Handler: func(ctx context.Context, exec aegis.ExecutionContext, input echoInput) (map[string]any, error) {
				return map[string]any{"message": input.Message}, nil
			},
		}),
	)

	kernel, err := aegis.NewBuilder(aegis.Config{}).
		WithModule(module).
		Build()
	if err != nil {
		t.Fatalf("build kernel: %v", err)
	}
	return kernel
}

func aegisMustBuildNoInputKernel(t *testing.T) *aegis.Kernel {
	t.Helper()

	module := aegis.NewModule(
		"echo",
		aegis.DefineOperation[struct{}, map[string]any](aegis.OperationSpec[struct{}, map[string]any]{
			Name: "echo.no_input",
			Handler: func(ctx context.Context, exec aegis.ExecutionContext, input struct{}) (map[string]any, error) {
				return map[string]any{"ok": true}, nil
			},
		}),
	)

	kernel, err := aegis.NewBuilder(aegis.Config{}).
		WithModule(module).
		Build()
	if err != nil {
		t.Fatalf("build kernel: %v", err)
	}
	return kernel
}
