package restadapter

import (
	"context"
	"net/http"

	"aegis"
)

type ContextBuilder func(ctx context.Context, r *http.Request) (context.Context, error)
type InputBinder[I any] func(r *http.Request) (I, error)
type ErrorEncoder func(w http.ResponseWriter, r *http.Request, err error)
type ResponseEncoder func(w http.ResponseWriter, status int, payload any) error

type Operation[I any] struct {
	Kernel          *aegis.Kernel
	Operation       string
	ContextBuilder  ContextBuilder
	InputBinder     InputBinder[I]
	SuccessStatus   int
	ErrorEncoder    ErrorEncoder
	ResponseEncoder ResponseEncoder
}

func NewJSONHandler[I any](cfg Operation[I]) http.Handler {
	if cfg.InputBinder == nil {
		cfg.InputBinder = NoInput[I]()
	}
	if cfg.SuccessStatus == 0 {
		cfg.SuccessStatus = http.StatusOK
	}
	if cfg.ErrorEncoder == nil {
		cfg.ErrorEncoder = DefaultErrorEncoder
	}
	if cfg.ResponseEncoder == nil {
		cfg.ResponseEncoder = WriteJSON
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cfg.Kernel == nil {
			cfg.ErrorEncoder(w, r, aegis.NewKernelError(aegis.CodeBootstrapFailed, "kernel is nil", nil))
			return
		}

		ctx := r.Context()
		if cfg.ContextBuilder != nil {
			var err error
			ctx, err = cfg.ContextBuilder(ctx, r)
			if err != nil {
				cfg.ErrorEncoder(w, r, err)
				return
			}
			if ctx == nil {
				ctx = r.Context()
			}
		}

		input, err := cfg.InputBinder(r)
		if err != nil {
			cfg.ErrorEncoder(w, r, err)
			return
		}

		output, err := cfg.Kernel.Execute(ctx, cfg.Operation, input)
		if err != nil {
			cfg.ErrorEncoder(w, r, err)
			return
		}

		if err := cfg.ResponseEncoder(w, cfg.SuccessStatus, output); err != nil {
			cfg.ErrorEncoder(w, r, err)
			return
		}
	})
}
