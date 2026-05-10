package picompat

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/easel/fizeau/internal/safefs"
)

// ProviderDefinition represents a provider in models.json.
type ProviderDefinition struct {
	Name        string                 `json:"name,omitempty"`
	Provider    string                 `json:"provider,omitempty"`
	BaseURL     string                 `json:"baseUrl,omitempty"`
	APIKey      string                 `json:"api_key,omitempty"`
	API         string                 `json:"api,omitempty"` // e.g., "openai-completions", "anthropic"
	Models      []string               `json:"models,omitempty"`
	Title       string                 `json:"title,omitempty"`
	Description string                 `json:"description,omitempty"`
	Extra       map[string]interface{} `json:"extra,omitempty"`
}

type providerDefinitionAlias ProviderDefinition

func (p *ProviderDefinition) UnmarshalJSON(data []byte) error {
	type modelObject struct {
		ID string `json:"id,omitempty"`
	}
	var raw struct {
		Name        string                 `json:"name,omitempty"`
		Provider    string                 `json:"provider,omitempty"`
		BaseURL     string                 `json:"baseUrl,omitempty"`
		APIKey      string                 `json:"api_key,omitempty"`
		API         string                 `json:"api,omitempty"`
		Models      []json.RawMessage      `json:"models,omitempty"`
		Title       string                 `json:"title,omitempty"`
		Description string                 `json:"description,omitempty"`
		Extra       map[string]interface{} `json:"extra,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	models := make([]string, 0, len(raw.Models))
	for _, entry := range raw.Models {
		var s string
		if err := json.Unmarshal(entry, &s); err == nil {
			if s != "" {
				models = append(models, s)
			}
			continue
		}
		var obj modelObject
		if err := json.Unmarshal(entry, &obj); err == nil {
			if obj.ID != "" {
				models = append(models, obj.ID)
				continue
			}
		}
		return fmt.Errorf("unsupported models entry %s", string(entry))
	}
	*p = ProviderDefinition{
		Name:        raw.Name,
		Provider:    raw.Provider,
		BaseURL:     raw.BaseURL,
		APIKey:      raw.APIKey,
		API:         raw.API,
		Models:      models,
		Title:       raw.Title,
		Description: raw.Description,
		Extra:       raw.Extra,
	}
	return nil
}

// ModelsConfig represents the pi models.json file.
type ModelsConfig struct {
	Providers []ProviderDefinition `json:"providers,omitempty"`
	Models    []ModelDefinition    `json:"models,omitempty"`
}

func (m *ModelsConfig) UnmarshalJSON(data []byte) error {
	var raw struct {
		Providers json.RawMessage   `json:"providers,omitempty"`
		Models    []ModelDefinition `json:"models,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var providers []ProviderDefinition
	if len(raw.Providers) > 0 {
		if raw.Providers[0] == '[' {
			if err := json.Unmarshal(raw.Providers, &providers); err != nil {
				return err
			}
		} else {
			var byName map[string]ProviderDefinition
			if err := json.Unmarshal(raw.Providers, &byName); err != nil {
				return err
			}
			names := make([]string, 0, len(byName))
			for name := range byName {
				names = append(names, name)
			}
			sort.Strings(names)
			providers = make([]ProviderDefinition, 0, len(names))
			for _, name := range names {
				prov := byName[name]
				if prov.Name == "" && prov.Provider == "" {
					prov.Name = name
				}
				providers = append(providers, prov)
			}
		}
	}
	m.Providers = providers
	m.Models = raw.Models
	return nil
}

// ModelDefinition represents a model entry in models.json.
type ModelDefinition struct {
	ID       string `json:"id,omitempty"`
	Provider string `json:"provider,omitempty"`
	Name     string `json:"name,omitempty"`
}

// LoadModels reads the pi models.json file.
func LoadModels(piDir string) (*ModelsConfig, error) {
	path := filepath.Join(piDir, "agent", "models.json")
	data, err := safefs.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg ModelsConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// GetProviderByName finds a provider definition by name.
func (m *ModelsConfig) GetProviderByName(name string) *ProviderDefinition {
	for i := range m.Providers {
		if m.Providers[i].Name == name || m.Providers[i].Provider == name {
			return &m.Providers[i]
		}
	}
	return nil
}
