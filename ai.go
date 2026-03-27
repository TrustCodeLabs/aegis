package aegis

type AIExposureSpec struct {
	Exposed              bool
	Exposure             string
	Title                string
	Summary              string
	Description          string
	UseCases             []string
	AvoidWhen            []string
	InvocationClass      string
	RequiresConfirmation bool
	SideEffects          []string
	Tags                 []string
}

type ModuleAISpec struct {
	Title       string
	Summary     string
	IntendedFor []string
	Skills      []SkillSpec
}

type SkillSpec struct {
	Name        string
	Title       string
	Description string
	Operations  []string
}

func cloneAIExposureSpec(in *AIExposureSpec) *AIExposureSpec {
	if in == nil {
		return nil
	}

	out := *in
	out.UseCases = cloneStringSlice(in.UseCases)
	out.AvoidWhen = cloneStringSlice(in.AvoidWhen)
	out.SideEffects = cloneStringSlice(in.SideEffects)
	out.Tags = cloneStringSlice(in.Tags)
	return &out
}

func cloneModuleAISpec(in *ModuleAISpec) *ModuleAISpec {
	if in == nil {
		return nil
	}

	out := *in
	out.IntendedFor = cloneStringSlice(in.IntendedFor)
	out.Skills = make([]SkillSpec, len(in.Skills))
	for index, skill := range in.Skills {
		out.Skills[index] = SkillSpec{
			Name:        skill.Name,
			Title:       skill.Title,
			Description: skill.Description,
			Operations:  cloneStringSlice(skill.Operations),
		}
	}
	return &out
}

func cloneStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
