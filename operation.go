package aegis

import (
	"context"
	"fmt"
)

type OperationDescriptor struct {
	Name                 string
	Version              string
	RequiredCapabilities []CapabilityRef
	RequiredPolicies     []PolicyRef
	Effects              []EffectSpec
	AI                   *AIExposureSpec
	Idempotent           bool
	Deterministic        bool
	InputSchema          Schema
	OutputSchema         Schema
}

type Operation interface {
	Descriptor() OperationDescriptor
	Validate(input any) error
	Execute(ctx context.Context, exec ExecutionContext, input any) (any, error)
}

type HandlerFunc[I, O any] func(ctx context.Context, exec ExecutionContext, input I) (O, error)
type ValidatorFunc[I any] func(input I) error

type OperationSpec[I, O any] struct {
	Name                 string
	Version              string
	RequiredCapabilities []CapabilityRef
	RequiredPolicies     []PolicyRef
	Effects              []EffectSpec
	AI                   *AIExposureSpec
	Idempotent           bool
	Deterministic        bool
	InputSchema          *Schema
	OutputSchema         *Schema
	Validate             ValidatorFunc[I]
	Handler              HandlerFunc[I, O]
}

type operation[I, O any] struct {
	spec       OperationSpec[I, O]
	descriptor OperationDescriptor
}

func (o operation[I, O]) Descriptor() OperationDescriptor {
	return cloneOperationDescriptor(o.descriptor)
}

func (o operation[I, O]) Validate(input any) error {
	typed, err := castInput[I](input)
	if err != nil {
		return err
	}

	if o.spec.Validate == nil {
		return nil
	}

	if err := o.spec.Validate(typed); err != nil {
		return newKernelError(
			CodeInvalidInput,
			fmt.Sprintf("operation %q rejected input", o.spec.Name),
			err,
		)
	}
	return nil
}

func (o operation[I, O]) Execute(ctx context.Context, exec ExecutionContext, input any) (any, error) {
	typed, err := castInput[I](input)
	if err != nil {
		return nil, err
	}

	if o.spec.Handler == nil {
		return nil, newKernelError(
			CodeBootstrapFailed,
			fmt.Sprintf("operation %q has no handler", o.spec.Name),
			nil,
		)
	}

	out, err := o.spec.Handler(ctx, exec, typed)
	if err != nil {
		return nil, err
	}
	return any(out), nil
}

func DefineOperation[I, O any](spec OperationSpec[I, O]) Operation {
	inputSchema := SchemaOf[I]()
	if spec.InputSchema != nil {
		inputSchema = *spec.InputSchema
	}

	outputSchema := SchemaOf[O]()
	if spec.OutputSchema != nil {
		outputSchema = *spec.OutputSchema
	}

	effects := cloneEffects(spec.Effects)
	for index := range effects {
		effects[index].Declared = true
	}

	return operation[I, O]{
		spec: spec,
		descriptor: OperationDescriptor{
			Name:                 spec.Name,
			Version:              spec.Version,
			RequiredCapabilities: cloneCapabilitySlice(spec.RequiredCapabilities),
			RequiredPolicies:     clonePolicyRefs(spec.RequiredPolicies),
			Effects:              effects,
			AI:                   cloneAIExposureSpec(spec.AI),
			Idempotent:           spec.Idempotent,
			Deterministic:        spec.Deterministic,
			InputSchema:          inputSchema,
			OutputSchema:         outputSchema,
		},
	}
}

func castInput[I any](input any) (I, error) {
	typed, ok := input.(I)
	if ok {
		return typed, nil
	}

	var zero I
	return zero, newKernelError(
		CodeInvalidInput,
		fmt.Sprintf("expected input of type %T, got %T", zero, input),
		nil,
	)
}

func cloneOperationDescriptor(in OperationDescriptor) OperationDescriptor {
	return OperationDescriptor{
		Name:                 in.Name,
		Version:              in.Version,
		RequiredCapabilities: cloneCapabilitySlice(in.RequiredCapabilities),
		RequiredPolicies:     clonePolicyRefs(in.RequiredPolicies),
		Effects:              cloneEffects(in.Effects),
		AI:                   cloneAIExposureSpec(in.AI),
		Idempotent:           in.Idempotent,
		Deterministic:        in.Deterministic,
		InputSchema:          in.InputSchema,
		OutputSchema:         in.OutputSchema,
	}
}

func clonePolicyRefs(in []PolicyRef) []PolicyRef {
	if len(in) == 0 {
		return nil
	}
	out := make([]PolicyRef, len(in))
	copy(out, in)
	return out
}

func cloneEffects(in []EffectSpec) []EffectSpec {
	if len(in) == 0 {
		return nil
	}
	out := make([]EffectSpec, len(in))
	for index, effect := range in {
		out[index] = normalizedEffectSpec(effect)
	}
	return out
}
