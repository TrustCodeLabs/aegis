package notes

import "aegis"

func readEffects() []aegis.EffectSpec {
	return []aegis.EffectSpec{
		{
			Name:        "storage.read." + ResourceName,
			Kind:        "storage.read",
			RequiredCap: "storage.read:" + ResourceName,
			Idempotent:  true,
			Metadata: map[string]any{
				"resource": ResourceName,
			},
		},
	}
}

func readWriteEffects() []aegis.EffectSpec {
	return []aegis.EffectSpec{
		{
			Name:        "storage.read." + ResourceName,
			Kind:        "storage.read",
			RequiredCap: "storage.read:" + ResourceName,
			Idempotent:  true,
			Metadata: map[string]any{
				"resource": ResourceName,
			},
		},
		{
			Name:        "storage.write." + ResourceName,
			Kind:        "storage.write",
			RequiredCap: "storage.write:" + ResourceName,
			Critical:    true,
			Idempotent:  false,
			Metadata: map[string]any{
				"resource": ResourceName,
			},
		},
	}
}

func deleteEffects() []aegis.EffectSpec {
	effects := readWriteEffects()
	effects = append(effects, aegis.EffectSpec{
		Name:        "storage.delete." + ResourceName,
		Kind:        "storage.delete",
		RequiredCap: "storage.write:" + ResourceName,
		Critical:    true,
		Idempotent:  false,
		Metadata: map[string]any{
			"resource": ResourceName,
		},
	})
	return effects
}
