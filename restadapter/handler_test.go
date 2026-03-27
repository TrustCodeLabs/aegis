package restadapter_test

import (
	"context"
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
