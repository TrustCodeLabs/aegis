package aegis

type Manifest struct {
	Name             string
	Version          string
	Status           string
	Description      string
	Dependencies     []string
	RequiredPolicies []PolicyRef
	AI               *ModuleAISpec
}

type Module interface {
	Manifest() Manifest
	Operations() []Operation
	Policies() []Policy
}

type StaticModule struct {
	manifest   Manifest
	operations []Operation
	policies   []Policy
}

func NewModule(name string, operations ...Operation) Module {
	return NewModuleWithManifest(Manifest{Name: name}, operations)
}

func NewModuleWithManifest(manifest Manifest, operations []Operation, policies ...Policy) Module {
	out := make([]Operation, len(operations))
	copy(out, operations)

	manifest.Dependencies = cloneStringSlice(manifest.Dependencies)
	manifest.RequiredPolicies = clonePolicyRefs(manifest.RequiredPolicies)
	manifest.AI = cloneModuleAISpec(manifest.AI)

	policiesOut := make([]Policy, len(policies))
	copy(policiesOut, policies)

	return StaticModule{
		manifest:   manifest,
		operations: out,
		policies:   policiesOut,
	}
}

func (m StaticModule) Manifest() Manifest {
	out := m.manifest
	out.Dependencies = cloneStringSlice(m.manifest.Dependencies)
	out.RequiredPolicies = clonePolicyRefs(m.manifest.RequiredPolicies)
	out.AI = cloneModuleAISpec(m.manifest.AI)
	return out
}

func (m StaticModule) Operations() []Operation {
	out := make([]Operation, len(m.operations))
	copy(out, m.operations)
	return out
}

func (m StaticModule) Policies() []Policy {
	out := make([]Policy, len(m.policies))
	copy(out, m.policies)
	return out
}
