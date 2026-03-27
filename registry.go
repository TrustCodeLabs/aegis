package aegis

import "fmt"

type registeredOperation struct {
	module     Manifest
	operation  Operation
	descriptor OperationDescriptor
}

type OperationRegistry struct {
	ops       map[string]registeredOperation
	modules   map[string]Manifest
	order     []string
	moduleOps map[string][]string
}

func NewOperationRegistry() *OperationRegistry {
	return &OperationRegistry{
		ops:       make(map[string]registeredOperation),
		modules:   make(map[string]Manifest),
		moduleOps: make(map[string][]string),
	}
}

func (r *OperationRegistry) RegisterModule(module Module) error {
	if module == nil {
		return newKernelError(CodeInvalidConfig, "module cannot be nil", nil)
	}

	manifest := module.Manifest()
	if manifest.Name == "" {
		return newKernelError(CodeInvalidConfig, "module name cannot be empty", nil)
	}
	if _, exists := r.modules[manifest.Name]; exists {
		return newKernelError(CodeDuplicateModule, fmt.Sprintf("module %q is already registered", manifest.Name), nil)
	}

	r.modules[manifest.Name] = manifest
	r.order = append(r.order, manifest.Name)

	for _, op := range module.Operations() {
		if op == nil {
			return newKernelError(CodeInvalidConfig, fmt.Sprintf("module %q contains a nil operation", manifest.Name), nil)
		}
		descriptor := op.Descriptor()
		if descriptor.Name == "" {
			return newKernelError(CodeInvalidConfig, fmt.Sprintf("module %q contains an operation without a name", manifest.Name), nil)
		}
		if _, exists := r.ops[descriptor.Name]; exists {
			return newKernelError(
				CodeDuplicateOperation,
				fmt.Sprintf("operation %q is already registered", descriptor.Name),
				nil,
			)
		}
		r.ops[descriptor.Name] = registeredOperation{
			module:     manifest,
			operation:  op,
			descriptor: descriptor,
		}
		r.moduleOps[manifest.Name] = append(r.moduleOps[manifest.Name], descriptor.Name)
	}

	return nil
}

func (r *OperationRegistry) Lookup(name string) (registeredOperation, bool) {
	op, ok := r.ops[name]
	return op, ok
}

func (r *OperationRegistry) Modules() []Manifest {
	out := make([]Manifest, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.modules[name])
	}
	return out
}

func (r *OperationRegistry) Entries() []registeredOperation {
	out := make([]registeredOperation, 0, len(r.ops))
	for _, name := range r.order {
		for _, opName := range r.moduleOps[name] {
			out = append(out, r.ops[opName])
		}
	}
	return out
}

func (r *OperationRegistry) ModuleOperationNames(name string) []string {
	return cloneStringSlice(r.moduleOps[name])
}
