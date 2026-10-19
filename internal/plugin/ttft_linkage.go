package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// The sibling opencode-go plugin publishes two read-only files next to our
// database: shared.json (its resolved settings, including the model alias
// table) and stats-<role>.json (per-model hourly counters, including TTFT).
// Plugin executors report provider names such as "cap-opencode-go" but the host
// hands usage plugins no TTFT for those requests, so the model view links the
// sibling's own measurements back in.
//
// Only whitelisted fields are decoded: model aliases and the per-hour
// ttft_nano/ttft_samples counters. The sibling also stores credentials in
// shared.json; those fields never enter any struct here, are never logged, and
// are never exposed through an HTTP response.
const (
	siblingSharedFileName = "shared.json"
	siblingStatsPrefix    = "stats-"
	siblingStatsSuffix    = ".json"
	siblingStatsVersion   = 1
	// siblingStatsRoles mirrors the sibling plugin's protocol faces.
	siblingStatsRoles = "chat,gemini,responses,messages"
)

// ttftLinkageSample is one (model, UTC hour) TTFT aggregate published by the
// sibling plugin.
type ttftLinkageSample struct {
	Samples uint64
	Nano    uint64
}

// ttftLinkage is an immutable snapshot of the sibling plugin's TTFT
// statistics. It is built outside the store actor and only read afterwards, so
// no lock is needed on the hot path.
type ttftLinkage struct {
	Enabled   bool
	Available bool
	Dir       string
	// Provider is the sibling plugin id taken from the directory name. Only
	// traffic from "cap-opencode-go" or "cap-opencode-go-*" providers may be
	// supplemented, so a same-named model served by another channel can never
	// borrow the sibling's samples.
	Provider  string
	Sources   []string
	WrittenAt time.Time
	Err       string
	// Index maps a lowercased model id or alias to the sibling model id.
	Index map[string]string
	// Hours maps a sibling model id to its UTC-hour TTFT samples.
	Hours map[string]map[int64]ttftLinkageSample
}

func (l *ttftLinkage) usable() bool {
	return l != nil && l.Enabled && l.Available && len(l.Hours) > 0
}

// lookup resolves one of our model names to the sibling model id.
func (l *ttftLinkage) lookup(model string) string {
	if l == nil || len(l.Index) == 0 {
		return ""
	}
	return l.Index[strings.ToLower(strings.TrimSpace(model))]
}

// sample returns the sibling's TTFT aggregate for a model id and UTC hour.
func (l *ttftLinkage) sample(modelID string, hour int64) (ttftLinkageSample, bool) {
	if l == nil || modelID == "" {
		return ttftLinkageSample{}, false
	}
	hours := l.Hours[modelID]
	if hours == nil {
		return ttftLinkageSample{}, false
	}
	value, ok := hours[hour]
	return value, ok
}

// ownsProvider reports whether a provider name belongs to the sibling plugin.
func (l *ttftLinkage) ownsProvider(provider string) bool {
	if l == nil || l.Provider == "" {
		return false
	}
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return false
	}
	return provider == l.Provider || strings.HasPrefix(provider, l.Provider+"-")
}

func (l *ttftLinkage) status() ttftSupplementStatus {
	if l == nil {
		return ttftSupplementStatus{Enabled: false, Available: false}
	}
	return ttftSupplementStatus{
		Enabled:   l.Enabled,
		Available: l.Available,
		Sources:   append([]string(nil), l.Sources...),
		WrittenAt: l.WrittenAt,
		Error:     l.Err,
	}
}

// ttftSupplementStatus is the response block that tells the dashboard whether
// the sibling linkage is active and how many samples it contributed.
type ttftSupplementStatus struct {
	Enabled   bool      `json:"enabled"`
	Available bool      `json:"available"`
	Sources   []string  `json:"sources,omitempty"`
	WrittenAt time.Time `json:"written_at,omitempty"`
	Models    int       `json:"models"`
	Samples   uint64    `json:"samples"`
	Error     string    `json:"error,omitempty"`
}

type siblingSharedFile struct {
	WrittenAt time.Time `json:"written_at"`
	Settings  struct {
		Models map[string]struct {
			Aliases []string `json:"aliases"`
		} `json:"models"`
	} `json:"settings"`
}

type siblingStatsFile struct {
	Version   int       `json:"version"`
	Role      string    `json:"role"`
	WrittenAt time.Time `json:"written_at"`
	Models    map[string]struct {
		Hours map[string]struct {
			TTFTSamples uint64 `json:"ttft_samples"`
			TTFTNano    uint64 `json:"ttft_nano"`
		} `json:"hours"`
	} `json:"models"`
}

type ttftFileStamp struct {
	Size    int64
	ModTime time.Time
}

// ttftLinkageCache memoizes parsed sibling files by size and modification time
// so a dashboard refresh never re-parses unchanged statistics.
type ttftLinkageCache struct {
	mu      sync.Mutex
	dir     string
	stamps  map[string]ttftFileStamp
	linkage *ttftLinkage
}

func newTTFTLinkageCache() *ttftLinkageCache {
	return &ttftLinkageCache{stamps: make(map[string]ttftFileStamp)}
}

// reset drops the memoized snapshot; callers invoke it after a config reload.
func (c *ttftLinkageCache) reset() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dir = ""
	c.stamps = make(map[string]ttftFileStamp)
	c.linkage = nil
}

// snapshot returns the current linkage snapshot, re-reading the sibling files
// only when one of them changed.
func (c *ttftLinkageCache) snapshot(config Config) *ttftLinkage {
	dir := config.resolvedTTFTSupplementDir()
	if !config.TTFTSupplement {
		return &ttftLinkage{Enabled: false, Dir: dir}
	}
	if c == nil {
		linkage, _ := loadTTFTLinkage(dir)
		return linkage
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	paths := siblingLinkagePaths(dir)
	stamps := make(map[string]ttftFileStamp, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		stamps[path] = ttftFileStamp{Size: info.Size(), ModTime: info.ModTime()}
	}
	if c.linkage != nil && c.dir == dir && sameTTFTStamps(c.stamps, stamps) {
		return c.linkage
	}
	linkage, _ := loadTTFTLinkage(dir)
	c.dir = dir
	c.stamps = stamps
	c.linkage = linkage
	return linkage
}

func sameTTFTStamps(left, right map[string]ttftFileStamp) bool {
	if len(left) != len(right) {
		return false
	}
	for path, stamp := range left {
		other, ok := right[path]
		if !ok || other != stamp {
			return false
		}
	}
	return true
}

func siblingLinkagePaths(dir string) []string {
	paths := []string{filepath.Join(dir, siblingSharedFileName)}
	for _, role := range strings.Split(siblingStatsRoles, ",") {
		paths = append(paths, filepath.Join(dir, siblingStatsPrefix+role+siblingStatsSuffix))
	}
	return paths
}

// resolvedTTFTSupplementDir returns the directory that holds the sibling
// plugin's published files. The default sits next to our own database, which is
// where a sibling plugin installed under the same CPA root writes.
func (c Config) resolvedTTFTSupplementDir() string {
	if trimmed := strings.TrimSpace(c.TTFTSupplementDir); trimmed != "" {
		return filepath.Clean(trimmed)
	}
	base := strings.TrimSpace(c.DataPath)
	if base == "" {
		base = defaultDataFileName
	}
	return filepath.Join(filepath.Dir(base), defaultSiblingStateDirName)
}

// defaultSiblingStateDirName is the opencode-go plugin's state directory name.
const defaultSiblingStateDirName = "cap-opencode-go"

// linkageProviderPrefix derives the sibling plugin id from its state directory.
// Only traffic whose provider matches that id (or one of its faces) is
// eligible for supplementation.
func linkageProviderPrefix(dir string) string {
	base := filepath.Base(filepath.Clean(dir))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return ""
	}
	return base
}

// loadTTFTLinkage reads the sibling snapshot from disk. Every failure is
// recorded in Err and degrades to "not available"; the dashboard keeps working.
func loadTTFTLinkage(dir string) (*ttftLinkage, map[string]ttftFileStamp) {
	linkage := &ttftLinkage{
		Enabled:  true,
		Dir:      dir,
		Provider: linkageProviderPrefix(dir),
		Index:    make(map[string]string),
		Hours:    make(map[string]map[int64]ttftLinkageSample),
	}
	stamps := make(map[string]ttftFileStamp)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		linkage.Err = fmt.Sprintf("sibling directory %s is unavailable", dir)
		return linkage, stamps
	}
	var problems []string

	sharedPath := filepath.Join(dir, siblingSharedFileName)
	if info, err := os.Stat(sharedPath); err == nil {
		stamps[sharedPath] = ttftFileStamp{Size: info.Size(), ModTime: info.ModTime()}
		raw, readErr := os.ReadFile(sharedPath)
		if readErr != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", siblingSharedFileName, readErr))
		} else {
			var shared siblingSharedFile
			if err := json.Unmarshal(raw, &shared); err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", siblingSharedFileName, err))
			} else {
				for id, spec := range shared.Settings.Models {
					id = strings.TrimSpace(id)
					if id == "" {
						continue
					}
					setDefaultIndex(linkage.Index, strings.ToLower(id), id)
					for _, alias := range spec.Aliases {
						alias = strings.ToLower(strings.TrimSpace(alias))
						if alias == "" {
							continue
						}
						setDefaultIndex(linkage.Index, alias, id)
					}
				}
				if shared.WrittenAt.After(linkage.WrittenAt) {
					linkage.WrittenAt = shared.WrittenAt
				}
				linkage.Sources = append(linkage.Sources, siblingSharedFileName)
			}
		}
	}

	for _, role := range strings.Split(siblingStatsRoles, ",") {
		name := siblingStatsPrefix + role + siblingStatsSuffix
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		stamps[path] = ttftFileStamp{Size: info.Size(), ModTime: info.ModTime()}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", name, readErr))
			continue
		}
		var file siblingStatsFile
		if err := json.Unmarshal(raw, &file); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		if file.Version != siblingStatsVersion {
			problems = append(problems, fmt.Sprintf("%s: unsupported version %d", name, file.Version))
			continue
		}
		for id, model := range file.Models {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			setDefaultIndex(linkage.Index, strings.ToLower(id), id)
			for hourKey, hour := range model.Hours {
				if hour.TTFTSamples == 0 {
					continue
				}
				bucket, err := time.ParseInLocation("2006-01-02T15", strings.TrimSpace(hourKey), time.UTC)
				if err != nil {
					continue
				}
				hours := linkage.Hours[id]
				if hours == nil {
					hours = make(map[int64]ttftLinkageSample)
					linkage.Hours[id] = hours
				}
				sample := hours[bucket.Unix()]
				sample.Samples = saturatingAdd(sample.Samples, hour.TTFTSamples)
				sample.Nano = saturatingAdd(sample.Nano, hour.TTFTNano)
				hours[bucket.Unix()] = sample
			}
		}
		if file.WrittenAt.After(linkage.WrittenAt) {
			linkage.WrittenAt = file.WrittenAt
		}
		linkage.Sources = append(linkage.Sources, name)
	}

	sort.Strings(linkage.Sources)
	linkage.Available = len(linkage.Hours) > 0
	if !linkage.Available && len(problems) == 0 {
		problems = append(problems, "sibling statistics are empty")
	}
	if len(problems) > 0 {
		linkage.Err = strings.Join(problems, "; ")
	}
	return linkage, stamps
}

func setDefaultIndex(index map[string]string, key, value string) {
	if index == nil || key == "" || value == "" {
		return
	}
	if _, exists := index[key]; !exists {
		index[key] = value
	}
}
