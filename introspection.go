package aegis

import (
	"context"
	"sort"
	"strings"
)

type ModuleInfo struct {
	Name         string
	Version      string
	Status       string
	Description  string
	Dependencies []string
	Operations   []string
	Policies     []string
	Capabilities []string
	AI           *ModuleAISpec
}

type OperationInfo struct {
	Name                 string
	Module               string
	Version              string
	InvocationClass      string
	Idempotent           bool
	Deterministic        bool
	RequiredPolicies     []string
	RequiredCapabilities []string
	Effects              []EffectSpecInfo
	Bindings             []BindingInfo
	AI                   *AIExposureSpec
	InputSchema          Schema
	OutputSchema         Schema
}

type PolicyInfo struct {
	ID          string
	Category    string
	Module      string
	Description string
	AppliesTo   []string
	Severity    string
}

type CapabilityGrantInfo struct {
	Module     string
	Capability string
	Granted    bool
	Source     string
	Conditions map[string]any
}

type BindingInfo struct {
	AdapterKind string
	Metadata    map[string]any
}

type TopologyNode struct {
	ID    string
	Kind  string
	Label string
}

type TopologyEdge struct {
	From string
	To   string
	Kind string
}

type TopologyGraph struct {
	Nodes []TopologyNode
	Edges []TopologyEdge
}

type IntrospectionFilter struct {
	Subject                Subject
	GrantedCapabilities    GrantedCapabilities
	UseGrantedCapabilities bool
	VisibilityTier         string
	Module                 string
	Operation              string
	AIOnly                 bool
}

type IntrospectionService interface {
	Modules(ctx context.Context, filter IntrospectionFilter) ([]ModuleInfo, error)
	Operations(ctx context.Context, filter IntrospectionFilter) ([]OperationInfo, error)
	Policies(ctx context.Context, filter IntrospectionFilter) ([]PolicyInfo, error)
	Capabilities(ctx context.Context, filter IntrospectionFilter) ([]CapabilityGrantInfo, error)
	Topology(ctx context.Context, filter IntrospectionFilter) (TopologyGraph, error)
	Effects(ctx context.Context, filter EffectQuery) ([]EffectRecord, error)
}

func (k *Kernel) Modules(ctx context.Context, filter IntrospectionFilter) ([]ModuleInfo, error) {
	modules := k.ops.Modules()
	out := make([]ModuleInfo, 0, len(modules))

	for _, manifest := range modules {
		if filter.Module != "" && manifest.Name != filter.Module {
			continue
		}

		operations := k.ops.ModuleOperationNames(manifest.Name)
		if filter.AIOnly {
			filtered := operations[:0]
			for _, opName := range operations {
				entry, ok := k.ops.Lookup(opName)
				if !ok || !aiVisible(entry.descriptor.AI, filter.VisibilityTier) {
					continue
				}
				filtered = append(filtered, opName)
			}
			operations = append([]string{}, filtered...)
			if len(operations) == 0 {
				continue
			}
		}

		policyIDs := make([]string, 0)
		capabilities := make([]string, 0)
		seenPolicies := map[string]struct{}{}
		seenCapabilities := map[string]struct{}{}

		for _, ref := range manifest.RequiredPolicies {
			if _, ok := seenPolicies[ref.ID]; ok || ref.ID == "" {
				continue
			}
			seenPolicies[ref.ID] = struct{}{}
			policyIDs = append(policyIDs, ref.ID)
		}

		for _, opName := range operations {
			entry, ok := k.ops.Lookup(opName)
			if !ok {
				continue
			}
			for _, ref := range entry.descriptor.RequiredPolicies {
				if _, ok := seenPolicies[ref.ID]; ok || ref.ID == "" {
					continue
				}
				seenPolicies[ref.ID] = struct{}{}
				policyIDs = append(policyIDs, ref.ID)
			}
			for _, capability := range entry.descriptor.RequiredCapabilities {
				key := capability.String()
				if _, ok := seenCapabilities[key]; ok || key == "" {
					continue
				}
				seenCapabilities[key] = struct{}{}
				capabilities = append(capabilities, key)
			}
			for _, effect := range entry.descriptor.Effects {
				key := effect.RequiredCap.String()
				if _, ok := seenCapabilities[key]; ok || key == "" {
					continue
				}
				seenCapabilities[key] = struct{}{}
				capabilities = append(capabilities, key)
			}
		}

		sort.Strings(operations)
		sort.Strings(policyIDs)
		sort.Strings(capabilities)

		out = append(out, ModuleInfo{
			Name:         manifest.Name,
			Version:      manifest.Version,
			Status:       manifest.Status,
			Description:  manifest.Description,
			Dependencies: cloneStringSlice(manifest.Dependencies),
			Operations:   operations,
			Policies:     policyIDs,
			Capabilities: capabilities,
			AI:           cloneModuleAISpec(manifest.AI),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})

	return out, nil
}

func (k *Kernel) Operations(ctx context.Context, filter IntrospectionFilter) ([]OperationInfo, error) {
	entries := k.ops.Entries()
	out := make([]OperationInfo, 0, len(entries))

	for _, entry := range entries {
		if filter.Module != "" && entry.module.Name != filter.Module {
			continue
		}
		if filter.Operation != "" && entry.descriptor.Name != filter.Operation {
			continue
		}
		if filter.AIOnly && !aiVisible(entry.descriptor.AI, filter.VisibilityTier) {
			continue
		}

		policies := make([]string, 0, len(entry.module.RequiredPolicies)+len(entry.descriptor.RequiredPolicies))
		for _, ref := range entry.module.RequiredPolicies {
			if ref.ID != "" {
				policies = append(policies, ref.ID)
			}
		}
		for _, ref := range entry.descriptor.RequiredPolicies {
			if ref.ID != "" {
				policies = append(policies, ref.ID)
			}
		}

		capabilities := make([]string, 0, len(entry.descriptor.RequiredCapabilities))
		for _, capability := range entry.descriptor.RequiredCapabilities {
			if capability == "" {
				continue
			}
			capabilities = append(capabilities, capability.String())
		}

		effects := make([]EffectSpecInfo, 0, len(entry.descriptor.Effects))
		for _, effect := range entry.descriptor.Effects {
			effects = append(effects, EffectSpecInfo{
				Name:       effect.Name,
				Kind:       effect.Kind,
				Critical:   effect.Critical,
				Idempotent: effect.Idempotent,
				Optional:   effect.Optional,
			})
		}

		out = append(out, OperationInfo{
			Name:                 entry.descriptor.Name,
			Module:               entry.module.Name,
			Version:              entry.descriptor.Version,
			InvocationClass:      invocationClass(entry.descriptor),
			Idempotent:           entry.descriptor.Idempotent,
			Deterministic:        entry.descriptor.Deterministic,
			RequiredPolicies:     policies,
			RequiredCapabilities: capabilities,
			Effects:              effects,
			Bindings:             bindingInfos(k.resources, entry.descriptor),
			AI:                   cloneAIExposureSpec(entry.descriptor.AI),
			InputSchema:          entry.descriptor.InputSchema,
			OutputSchema:         entry.descriptor.OutputSchema,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})

	return out, nil
}

func (k *Kernel) Policies(ctx context.Context, filter IntrospectionFilter) ([]PolicyInfo, error) {
	policies := k.policyRegistry.All()
	out := make([]PolicyInfo, 0, len(policies))
	for _, policy := range policies {
		metadata := policy.Metadata()
		if filter.Module != "" && metadata.Module != filter.Module {
			continue
		}
		out = append(out, PolicyInfo{
			ID:          policy.ID(),
			Category:    metadata.Category,
			Module:      metadata.Module,
			Description: metadata.Description,
			AppliesTo:   cloneStringSlice(metadata.AppliesTo),
			Severity:    metadata.Severity,
		})
	}
	return out, nil
}

func (k *Kernel) Capabilities(ctx context.Context, filter IntrospectionFilter) ([]CapabilityGrantInfo, error) {
	ops, err := k.Operations(ctx, filter)
	if err != nil {
		return nil, err
	}

	resolution := ResolveCapabilities(filter.Subject, filter.GrantedCapabilities, filter.UseGrantedCapabilities)

	out := make([]CapabilityGrantInfo, 0)
	seen := map[string]struct{}{}
	for _, op := range ops {
		for _, capability := range op.RequiredCapabilities {
			key := op.Module + ":" + capability
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			isGranted := resolution.Effective.Has(CapabilityRef(capability))
			out = append(out, CapabilityGrantInfo{
				Module:     op.Module,
				Capability: capability,
				Granted:    isGranted,
				Source:     string(resolution.Source),
				Conditions: map[string]any{},
			})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Module == out[j].Module {
			return out[i].Capability < out[j].Capability
		}
		return out[i].Module < out[j].Module
	})
	return out, nil
}

func (k *Kernel) Topology(ctx context.Context, filter IntrospectionFilter) (TopologyGraph, error) {
	modules, err := k.Modules(ctx, filter)
	if err != nil {
		return TopologyGraph{}, err
	}
	ops, err := k.Operations(ctx, filter)
	if err != nil {
		return TopologyGraph{}, err
	}

	graph := TopologyGraph{}
	nodeSeen := map[string]struct{}{}
	edgeSeen := map[string]struct{}{}

	addNode := func(node TopologyNode) {
		if _, ok := nodeSeen[node.ID]; ok {
			return
		}
		nodeSeen[node.ID] = struct{}{}
		graph.Nodes = append(graph.Nodes, node)
	}
	addEdge := func(edge TopologyEdge) {
		key := edge.From + "|" + edge.To + "|" + edge.Kind
		if _, ok := edgeSeen[key]; ok {
			return
		}
		edgeSeen[key] = struct{}{}
		graph.Edges = append(graph.Edges, edge)
	}

	for _, module := range modules {
		moduleID := "module:" + module.Name
		addNode(TopologyNode{ID: moduleID, Kind: "module", Label: module.Name})

		for _, dependency := range module.Dependencies {
			dependencyID := "module:" + dependency
			addNode(TopologyNode{ID: dependencyID, Kind: "module", Label: dependency})
			addEdge(TopologyEdge{From: moduleID, To: dependencyID, Kind: "dependency"})
		}
	}

	for _, op := range ops {
		moduleID := "module:" + op.Module
		opID := "operation:" + op.Name
		addNode(TopologyNode{ID: opID, Kind: "operation", Label: op.Name})
		addEdge(TopologyEdge{From: moduleID, To: opID, Kind: "contains"})

		for _, policyID := range op.RequiredPolicies {
			policyNodeID := "policy:" + policyID
			addNode(TopologyNode{ID: policyNodeID, Kind: "policy", Label: policyID})
			addEdge(TopologyEdge{From: opID, To: policyNodeID, Kind: "requires_policy"})
		}
		for _, capability := range op.RequiredCapabilities {
			capNodeID := "capability:" + capability
			addNode(TopologyNode{ID: capNodeID, Kind: "capability", Label: capability})
			addEdge(TopologyEdge{From: opID, To: capNodeID, Kind: "requires_capability"})
		}
		for _, effect := range op.Effects {
			effectNodeID := "effect:" + effect.Name
			addNode(TopologyNode{ID: effectNodeID, Kind: "effect", Label: effect.Name})
			addEdge(TopologyEdge{From: opID, To: effectNodeID, Kind: "declares_effect"})
		}
	}

	sort.Slice(graph.Nodes, func(i, j int) bool { return graph.Nodes[i].ID < graph.Nodes[j].ID })
	sort.Slice(graph.Edges, func(i, j int) bool {
		if graph.Edges[i].From == graph.Edges[j].From {
			if graph.Edges[i].To == graph.Edges[j].To {
				return graph.Edges[i].Kind < graph.Edges[j].Kind
			}
			return graph.Edges[i].To < graph.Edges[j].To
		}
		return graph.Edges[i].From < graph.Edges[j].From
	})

	return graph, nil
}

func (k *Kernel) Effects(ctx context.Context, filter EffectQuery) ([]EffectRecord, error) {
	return k.effectStore.Query(filter)
}

func aiVisible(spec *AIExposureSpec, tier string) bool {
	if spec == nil || !spec.Exposed {
		return false
	}
	return visibilityRank(spec.Exposure) <= visibilityRank(tierOrDefault(tier))
}

func tierOrDefault(in string) string {
	if in == "" {
		return "internal"
	}
	return strings.ToLower(in)
}

func visibilityRank(in string) int {
	switch strings.ToLower(in) {
	case "public":
		return 1
	case "workspace":
		return 2
	case "internal":
		return 3
	case "restricted":
		return 4
	default:
		return 5
	}
}

func invocationClass(descriptor OperationDescriptor) string {
	if descriptor.AI != nil && descriptor.AI.InvocationClass != "" {
		return descriptor.AI.InvocationClass
	}
	return "read"
}

func bindingInfos(resources *ResourceManager, descriptor OperationDescriptor) []BindingInfo {
	out := make([]BindingInfo, 0)
	seen := map[string]struct{}{}
	for _, effect := range descriptor.Effects {
		resource, ok := effect.Metadata["resource"].(string)
		if !ok || resource == "" {
			continue
		}

		adapterKind := "resource"
		switch {
		case strings.HasPrefix(effect.Kind, "storage."):
			adapterKind = "storage"
		case strings.HasPrefix(effect.Kind, "sql."):
			adapterKind = "sql"
		case strings.HasPrefix(effect.Kind, "cache."):
			adapterKind = "cache"
		case strings.HasPrefix(effect.Kind, "http."):
			adapterKind = "http"
		}

		key := adapterKind + ":" + resource
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		metadata := map[string]any{
			"resource": resource,
		}
		if resources != nil {
			if binding, ok := resources.StorageBindingInfo(resource); ok {
				metadata["driver"] = binding.Driver
				metadata["provider"] = binding.Provider
				metadata["layered"] = binding.Layered
				metadata["multi_tenant"] = binding.MultiTenant
				metadata["hot_swappable"] = binding.HotSwappable
				if len(binding.Tenants) > 0 {
					metadata["tenants"] = cloneStringSlice(binding.Tenants)
				}
			}
		}

		out = append(out, BindingInfo{
			AdapterKind: adapterKind,
			Metadata:    metadata,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].AdapterKind == out[j].AdapterKind {
			left, _ := out[i].Metadata["resource"].(string)
			right, _ := out[j].Metadata["resource"].(string)
			return left < right
		}
		return out[i].AdapterKind < out[j].AdapterKind
	})
	return out
}
