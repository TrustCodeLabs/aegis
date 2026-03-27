package aegis

import "context"

type MCPTool struct {
	Name                 string
	Title                string
	Description          string
	InputSchema          Schema
	OutputSchema         Schema
	Exposure             string
	InvocationClass      string
	RequiresConfirmation bool
	SideEffects          []string
	RequiredCapabilities []string
	RequiredPolicies     []string
	Tags                 []string
	Idempotent           bool
}

func (k *Kernel) MCPTools(ctx context.Context, filter IntrospectionFilter) ([]MCPTool, error) {
	filter.AIOnly = true
	if filter.VisibilityTier == "" {
		filter.VisibilityTier = "internal"
	}

	operations, err := k.Operations(ctx, filter)
	if err != nil {
		return nil, err
	}

	out := make([]MCPTool, 0, len(operations))
	for _, op := range operations {
		description := op.Name
		title := op.Name
		exposure := "internal"
		var requiresConfirmation bool
		var tags []string
		if op.AI != nil {
			if op.AI.Title != "" {
				title = op.AI.Title
			}
			if op.AI.Description != "" {
				description = op.AI.Description
			} else if op.AI.Summary != "" {
				description = op.AI.Summary
			}
			if op.AI.Exposure != "" {
				exposure = op.AI.Exposure
			}
			requiresConfirmation = op.AI.RequiresConfirmation
			tags = cloneStringSlice(op.AI.Tags)
		}

		out = append(out, MCPTool{
			Name:                 op.Name,
			Title:                title,
			Description:          description,
			InputSchema:          op.InputSchema,
			OutputSchema:         op.OutputSchema,
			Exposure:             exposure,
			InvocationClass:      op.InvocationClass,
			RequiresConfirmation: requiresConfirmation,
			SideEffects:          sideEffectsFor(op),
			RequiredCapabilities: cloneStringSlice(op.RequiredCapabilities),
			RequiredPolicies:     cloneStringSlice(op.RequiredPolicies),
			Tags:                 tags,
			Idempotent:           op.Idempotent,
		})
	}

	return out, nil
}

func (k *Kernel) InvokeTool(ctx context.Context, toolName string, input any) (any, error) {
	return k.Execute(ctx, toolName, input)
}
