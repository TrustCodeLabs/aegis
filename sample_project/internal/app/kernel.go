package app

import (
	"path/filepath"

	"aegis"
	"aegis/drivers/localstorage"
	"aegis/sample_project/internal/notes"
)

func buildKernel(mode string) (*aegis.Kernel, error) {
	registry := aegis.NewDriverRegistry()
	if err := localstorage.Register(registry); err != nil {
		return nil, err
	}

	return aegis.NewBuilder(aegis.Config{
		Resources: aegis.ResourcesConfig{
			Storage: map[string]aegis.StorageBinding{
				notes.ResourceName: storageBindingForMode(mode),
			},
		},
	}).
		WithDriverRegistry(registry).
		WithModule(notes.BuildModule()).
		WithHighAssuranceEffects(true).
		Build()
}

func storageBindingForMode(mode string) aegis.StorageBinding {
	switch NormalizeStorageMode(mode) {
	case StorageModeDirect:
		return aegis.StorageBinding{
			HotSwappable: true,
			Tenant: map[string]aegis.StorageBinding{
				"team-a": {
					Driver:   "local",
					Provider: "team-a-direct",
					Config: map[string]any{
						"root": filepath.Clean("./runtime/storage/direct/team-a"),
					},
				},
				"team-b": {
					Driver:   "local",
					Provider: "team-b-direct",
					Config: map[string]any{
						"root": filepath.Clean("./runtime/storage/direct/team-b"),
					},
				},
			},
		}
	default:
		return aegis.StorageBinding{
			HotSwappable: true,
			Tenant: map[string]aegis.StorageBinding{
				"team-a": layeredTenantBinding("team-a"),
				"team-b": layeredTenantBinding("team-b"),
			},
		}
	}
}

func layeredTenantBinding(tenant string) aegis.StorageBinding {
	return aegis.StorageBinding{
		Driver:   "layered",
		Provider: tenant + "-layered",
		Layers: []aegis.StorageBinding{
			{
				Driver:   "local",
				Provider: tenant + "-cache",
				Config: map[string]any{
					"root": filepath.Clean("./runtime/storage/layered/" + tenant + "/cache"),
				},
			},
			{
				Driver:   "local",
				Provider: tenant + "-durable",
				Config: map[string]any{
					"root": filepath.Clean("./runtime/storage/layered/" + tenant + "/durable"),
				},
			},
		},
	}
}
