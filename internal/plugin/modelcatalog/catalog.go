// Package modelcatalog stores the trimmed models.dev price catalog used by the
// dashboard. The internal format is deliberately small and stable: upstream
// keys are remapped, absent values are omitted, and the trimmed document
// carries a schema version so a future format change is rejected instead of
// silently misread.
package modelcatalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SchemaVersion is the internal (trimmed) catalog format version. Parsing a
// document with any other version fails.
const SchemaVersion = 1

// Price is the list price of one model in USD per 1,000,000 tokens. Missing
// fields stay nil so "absent" and "zero" remain distinguishable.
type Price struct {
	Input      *float64 `json:"i,omitempty"`
	Output     *float64 `json:"o,omitempty"`
	CacheRead  *float64 `json:"cr,omitempty"`
	CacheWrite *float64 `json:"cw,omitempty"`
}

// Priced reports whether the record has a list price. Only input or output
// matter: a record that only carries cache rates is not a usable price.
func (p Price) Priced() bool {
	return p.Input != nil || p.Output != nil
}

// Free reports whether the record is an explicit zero price.
func (p Price) Free() bool {
	return p.Priced() && numericValue(p.Input) == 0 && numericValue(p.Output) == 0
}

// Complete reports whether every rate is present.
func (p Price) Complete() bool {
	return p.Input != nil && p.Output != nil && p.CacheRead != nil && p.CacheWrite != nil
}

func numericValue(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

// Model is one model entry of the trimmed catalog.
type Model struct {
	Family string `json:"f,omitempty"`
	Cost   Price  `json:"c,omitzero"`
}

// Provider is one provider entry of the trimmed catalog.
type Provider struct {
	Name   string           `json:"n,omitempty"`
	API    string           `json:"api,omitempty"`
	Models map[string]Model `json:"models,omitempty"`
}

// Snapshot is a parsed trimmed catalog.
type Snapshot struct {
	SchemaVersion int
	GeneratedAt   time.Time
	Providers     map[string]Provider
}

// Stats summarises catalog coverage for status reporting.
type Stats struct {
	Providers int
	Models    int
	Priced    int
	Free      int
}

// ComputeStats counts providers, models, priced models and explicit free
// models in one pass.
func (s *Snapshot) ComputeStats() Stats {
	var stats Stats
	if s == nil {
		return stats
	}
	for _, provider := range s.Providers {
		stats.Providers++
		for _, model := range provider.Models {
			stats.Models++
			if model.Cost.Priced() {
				stats.Priced++
			}
			if model.Cost.Free() {
				stats.Free++
			}
		}
	}
	return stats
}

// document mirrors the trimmed JSON. GeneratedAt is decoded as text so a
// malformed timestamp degrades to the zero time instead of failing the whole
// document.
type document struct {
	SchemaVersion int                 `json:"schemaVersion"`
	GeneratedAt   string              `json:"generatedAt"`
	Providers     map[string]Provider `json:"providers"`
}

// Parse decodes a trimmed catalog document.
func Parse(raw []byte) (*Snapshot, error) {
	var doc document
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode model catalog: %w", err)
	}
	if doc.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("unsupported model catalog schemaVersion %d", doc.SchemaVersion)
	}
	if len(doc.Providers) == 0 {
		return nil, fmt.Errorf("model catalog has no providers")
	}
	snapshot := &Snapshot{SchemaVersion: SchemaVersion, Providers: make(map[string]Provider, len(doc.Providers))}
	if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(doc.GeneratedAt)); err == nil {
		snapshot.GeneratedAt = parsed.UTC()
	}
	models := 0
	for providerID, provider := range doc.Providers {
		id := strings.TrimSpace(providerID)
		if id == "" {
			continue
		}
		cleaned := Provider{Name: provider.Name, API: provider.API, Models: make(map[string]Model, len(provider.Models))}
		for modelID, model := range provider.Models {
			key := strings.TrimSpace(modelID)
			if key == "" {
				continue
			}
			cleaned.Models[key] = model
			models++
		}
		snapshot.Providers[id] = cleaned
	}
	if len(snapshot.Providers) == 0 || models == 0 {
		return nil, fmt.Errorf("model catalog has no models")
	}
	return snapshot, nil
}

// upstreams below mirror the models.dev /api.json shape. Unknown upstream
// fields are ignored.
type upstreamProvider struct {
	ID     string                     `json:"id"`
	Name   string                     `json:"name"`
	API    string                     `json:"api"`
	Models map[string]json.RawMessage `json:"models"`
}

type upstreamModel struct {
	ID     string       `json:"id"`
	Name   string       `json:"name"`
	Family string       `json:"family"`
	Cost   upstreamCost `json:"cost"`
}

type upstreamCost struct {
	Input      *float64 `json:"input"`
	Output     *float64 `json:"output"`
	CacheRead  *float64 `json:"cache_read"`
	CacheWrite *float64 `json:"cache_write"`
}

// Trim converts a raw models.dev payload into the trimmed catalog document
// bytes. generatedAt records when the snapshot was produced because the
// upstream payload carries no timestamp.
func Trim(raw []byte, generatedAt time.Time) ([]byte, *Snapshot, error) {
	var upstream map[string]json.RawMessage
	if err := json.Unmarshal(raw, &upstream); err != nil {
		return nil, nil, fmt.Errorf("decode models.dev payload: %w", err)
	}
	snapshot := &Snapshot{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   generatedAt.UTC(),
		Providers:     make(map[string]Provider, len(upstream)),
	}
	for providerKey, providerRaw := range upstream {
		var provider upstreamProvider
		if err := json.Unmarshal(providerRaw, &provider); err != nil {
			// A single malformed provider must not discard the catalog.
			continue
		}
		providerID := strings.TrimSpace(provider.ID)
		if providerID == "" {
			providerID = strings.TrimSpace(providerKey)
		}
		if providerID == "" || len(provider.Models) == 0 {
			continue
		}
		entry := Provider{Name: strings.TrimSpace(provider.Name), API: strings.TrimSpace(provider.API), Models: make(map[string]Model, len(provider.Models))}
		if entry.Name == "" {
			entry.Name = providerID
		}
		for modelKey, modelRaw := range provider.Models {
			var model upstreamModel
			if err := json.Unmarshal(modelRaw, &model); err != nil {
				continue
			}
			modelID := strings.TrimSpace(model.ID)
			if modelID == "" {
				modelID = strings.TrimSpace(modelKey)
			}
			if modelID == "" {
				continue
			}
			trimmed := Model{Family: strings.TrimSpace(model.Family)}
			if model.Cost.Input != nil || model.Cost.Output != nil || model.Cost.CacheRead != nil || model.Cost.CacheWrite != nil {
				trimmed.Cost = Price{
					Input:      model.Cost.Input,
					Output:     model.Cost.Output,
					CacheRead:  model.Cost.CacheRead,
					CacheWrite: model.Cost.CacheWrite,
				}
			}
			entry.Models[modelID] = trimmed
		}
		if len(entry.Models) == 0 {
			continue
		}
		snapshot.Providers[providerID] = entry
	}
	if len(snapshot.Providers) == 0 {
		return nil, nil, fmt.Errorf("models.dev payload produced no providers")
	}
	encoded, err := json.Marshal(document{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   snapshot.GeneratedAt.Format(time.RFC3339),
		Providers:     snapshot.Providers,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("encode model catalog: %w", err)
	}
	return encoded, snapshot, nil
}

// Valid reports whether the snapshot can serve lookups.
func (s *Snapshot) Valid() bool {
	if s == nil || len(s.Providers) == 0 {
		return false
	}
	for _, provider := range s.Providers {
		if len(provider.Models) > 0 {
			return true
		}
	}
	return false
}
