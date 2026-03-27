package restadapter

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"aegis"
)

func NoInput[I any]() InputBinder[I] {
	var zero I
	return StaticInput(zero)
}

func StaticInput[I any](input I) InputBinder[I] {
	return func(r *http.Request) (I, error) {
		return input, nil
	}
}

func DecodeJSONBody[I any](r *http.Request) (I, error) {
	var input I
	if r.Body == nil {
		return input, aegis.NewKernelError(aegis.CodeInvalidInput, "request body is required", nil)
	}
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		if err == io.EOF {
			return input, aegis.NewKernelError(aegis.CodeInvalidInput, "request body is required", nil)
		}
		return input, aegis.NewKernelError(aegis.CodeInvalidInput, "invalid json request body", err)
	}

	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return input, aegis.NewKernelError(aegis.CodeInvalidInput, "request body must contain a single JSON value", nil)
		}
		return input, aegis.NewKernelError(aegis.CodeInvalidInput, "invalid json request body", err)
	}

	return input, nil
}

func WriteJSON(w http.ResponseWriter, status int, payload any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(payload); err != nil {
		return fmt.Errorf("encode json response: %w", err)
	}
	return nil
}
