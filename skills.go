package aegis

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (k *Kernel) GenerateSkillsMarkdown(ctx context.Context, filter IntrospectionFilter) (string, error) {
	filter.AIOnly = true
	if filter.VisibilityTier == "" {
		filter.VisibilityTier = "internal"
	}

	modules, err := k.Modules(ctx, IntrospectionFilter{
		Module:         filter.Module,
		VisibilityTier: filter.VisibilityTier,
		AIOnly:         true,
	})
	if err != nil {
		return "", err
	}

	operations, err := k.Operations(ctx, filter)
	if err != nil {
		return "", err
	}

	opsByModule := map[string][]OperationInfo{}
	opsByName := map[string]OperationInfo{}
	for _, op := range operations {
		opsByModule[op.Module] = append(opsByModule[op.Module], op)
		opsByName[op.Name] = op
	}

	var builder strings.Builder
	builder.WriteString("# Skills\n\n")

	wroteSkill := false
	for _, module := range modules {
		moduleOps := opsByModule[module.Name]
		if len(moduleOps) == 0 {
			continue
		}

		grouped := map[string]struct{}{}
		if module.AI != nil {
			for _, skill := range module.AI.Skills {
				selected := make([]OperationInfo, 0, len(skill.Operations))
				for _, opName := range skill.Operations {
					op, ok := opsByName[opName]
					if !ok {
						continue
					}
					grouped[opName] = struct{}{}
					selected = append(selected, op)
				}
				if len(selected) == 0 {
					continue
				}
				renderSkill(&builder, module, skill.Name, skill.Title, skill.Description, selected)
				wroteSkill = true
			}
		}

		fallback := make([]OperationInfo, 0)
		for _, op := range moduleOps {
			if _, ok := grouped[op.Name]; ok {
				continue
			}
			fallback = append(fallback, op)
		}
		if len(fallback) > 0 {
			title := module.Name + " operations"
			description := module.Description
			if module.AI != nil {
				if module.AI.Title != "" {
					title = module.AI.Title
				}
				if module.AI.Summary != "" {
					description = module.AI.Summary
				}
			}
			renderSkill(&builder, module, module.Name+".default", title, description, fallback)
			wroteSkill = true
		}
	}

	if !wroteSkill {
		builder.WriteString("_No AI-exposed operations available._\n")
	}

	return builder.String(), nil
}

func (k *Kernel) WriteSkillsMarkdown(ctx context.Context, filter IntrospectionFilter, outputPath string) error {
	if outputPath == "" {
		outputPath = filepath.Join("generated", "SKILLS.md")
	}

	content, err := k.GenerateSkillsMarkdown(ctx, filter)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(outputPath, []byte(content), 0o644)
}

func renderSkill(builder *strings.Builder, module ModuleInfo, name, title, description string, operations []OperationInfo) {
	sort.Slice(operations, func(i, j int) bool { return operations[i].Name < operations[j].Name })

	if name == "" {
		name = module.Name + ".default"
	}
	if title == "" {
		title = name
	}
	if description == "" {
		description = title
	}

	useCases := uniqueStrings(nil)
	avoidWhen := uniqueStrings(nil)

	for _, op := range operations {
		if op.AI == nil {
			continue
		}
		useCases = uniqueStrings(append(useCases, op.AI.UseCases...))
		avoidWhen = uniqueStrings(append(avoidWhen, op.AI.AvoidWhen...))
	}

	builder.WriteString("## ")
	builder.WriteString(name)
	builder.WriteString("\n\n")
	builder.WriteString("**Description**  \n")
	builder.WriteString(description)
	builder.WriteString("\n\n")

	builder.WriteString("**Use when**\n")
	if len(useCases) == 0 {
		builder.WriteString("- a caller needs one of the operations in this skill\n")
	} else {
		for _, item := range useCases {
			builder.WriteString("- ")
			builder.WriteString(item)
			builder.WriteString("\n")
		}
	}
	builder.WriteString("\n")

	builder.WriteString("**Do not use when**\n")
	if len(avoidWhen) == 0 {
		builder.WriteString("- the action falls outside the declared operation boundaries\n")
	} else {
		for _, item := range avoidWhen {
			builder.WriteString("- ")
			builder.WriteString(item)
			builder.WriteString("\n")
		}
	}
	builder.WriteString("\n")

	builder.WriteString("**Operations**\n")
	for _, op := range operations {
		builder.WriteString("- `")
		builder.WriteString(op.Name)
		builder.WriteString("`\n")
	}
	builder.WriteString("\n")

	for _, op := range operations {
		builder.WriteString("### ")
		builder.WriteString(op.Name)
		builder.WriteString("\n")

		builder.WriteString("- Title: ")
		if op.AI != nil && op.AI.Title != "" {
			builder.WriteString(op.AI.Title)
		} else {
			builder.WriteString(op.Name)
		}
		builder.WriteString("\n")

		builder.WriteString("- Summary: ")
		if op.AI != nil && op.AI.Summary != "" {
			builder.WriteString(op.AI.Summary)
		} else {
			builder.WriteString(op.Name)
		}
		builder.WriteString("\n")

		builder.WriteString("- Class: ")
		builder.WriteString(op.InvocationClass)
		builder.WriteString("\n")

		builder.WriteString("- Input: ")
		builder.WriteString(schemaSummary(op.InputSchema))
		builder.WriteString("\n")

		builder.WriteString("- Output: ")
		builder.WriteString(schemaSummary(op.OutputSchema))
		builder.WriteString("\n")

		builder.WriteString("- Side effects:\n")
		sideEffects := sideEffectsFor(op)
		for _, item := range sideEffects {
			builder.WriteString("  - ")
			builder.WriteString(item)
			builder.WriteString("\n")
		}

		builder.WriteString("- Confirmation required: ")
		if op.AI != nil && op.AI.RequiresConfirmation {
			builder.WriteString("yes\n")
		} else {
			builder.WriteString("no\n")
		}
		builder.WriteString("\n")
	}
}

func sideEffectsFor(op OperationInfo) []string {
	if op.AI != nil && len(op.AI.SideEffects) > 0 {
		return uniqueStrings(cloneStringSlice(op.AI.SideEffects))
	}

	out := make([]string, 0, len(op.Effects))
	for _, effect := range op.Effects {
		out = append(out, effect.Name)
	}
	if len(out) == 0 {
		out = append(out, "none declared")
	}
	return uniqueStrings(out)
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
