package aegis

type Config struct {
	Resources ResourcesConfig `json:"resources"`
}

type ResourcesConfig struct {
	Storage map[string]StorageBinding `json:"storage"`
}

type StorageBinding struct {
	Driver       string                    `json:"driver"`
	Provider     string                    `json:"provider,omitempty"`
	Config       map[string]any            `json:"config,omitempty"`
	Layers       []StorageBinding          `json:"layers,omitempty"`
	Tenant       map[string]StorageBinding `json:"tenant,omitempty"`
	HotSwappable bool                      `json:"hot_swappable,omitempty"`
}

func (b StorageBinding) LookupString(key string) (string, bool) {
	if len(b.Config) == 0 {
		return "", false
	}

	raw, ok := b.Config[key]
	if !ok {
		return "", false
	}

	value, ok := raw.(string)
	return value, ok
}

func (b StorageBinding) ProviderName(fallback string) string {
	if b.Provider != "" {
		return b.Provider
	}
	return fallback
}

func (b StorageBinding) clone() StorageBinding {
	out := StorageBinding{
		Driver:       b.Driver,
		Provider:     b.Provider,
		Config:       cloneMap(b.Config),
		HotSwappable: b.HotSwappable,
	}
	if len(b.Layers) > 0 {
		out.Layers = make([]StorageBinding, len(b.Layers))
		for index, layer := range b.Layers {
			out.Layers[index] = layer.clone()
		}
	}
	if len(b.Tenant) > 0 {
		out.Tenant = make(map[string]StorageBinding, len(b.Tenant))
		for tenant, binding := range b.Tenant {
			out.Tenant[tenant] = binding.clone()
		}
	}
	return out
}
