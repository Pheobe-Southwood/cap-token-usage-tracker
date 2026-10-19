package modelcatalog

import (
	"net/url"
	"sort"
	"strings"
)

// Match kinds reported to the dashboard so a resolved price can be explained.
const (
	// MatchHint is a hit inside a provider derived from the usage source or
	// channel name.
	MatchHint = "provider"
	// MatchVendor is a hit through the slash vendor prefix table.
	MatchVendor = "vendor"
	// MatchGlobal is a hit in the global normalized index.
	MatchGlobal = "global"
	// MatchVariant is a hit found after stripping a reasoning/effort suffix.
	MatchVariant = "variant"
)

// Match is one resolved catalog entry.
type Match struct {
	ProviderID string
	ModelID    string
	Model      Model
	Kind       string
}

// firstPartyPriority mirrors the upstream first-party ranking used as the
// primary tie-breaker when several providers list the same model id.
var firstPartyPriority = []string{
	"openai",
	"anthropic",
	"google",
	"google-vertex",
	"deepseek",
	"moonshotai",
	"alibaba",
	"alibaba-cn",
	"zhipuai",
	"zai",
	"xai",
	"mistral",
	"minimax",
	"minimax-cn",
	"volcengine",
	"stepfun",
	"meta",
	"cohere",
	"groq",
	"cerebras",
	"togetherai",
	"fireworks-ai",
	"openrouter",
}

// vendorPrefixToProvider maps aggregator slash prefixes to a first-party
// provider id.
var vendorPrefixToProvider = map[string]string{
	"openai":     "openai",
	"anthropic":  "anthropic",
	"google":     "google",
	"deepseek":   "deepseek",
	"moonshotai": "moonshotai",
	"qwen":       "alibaba",
	"x-ai":       "xai",
	"z-ai":       "zai",
	"mistralai":  "mistral",
	"meta-llama": "meta",
}

// exactHostProviders covers providers whose upstream catalog entry omits the
// api field, so a base URL cannot be mapped through the catalog alone.
var exactHostProviders = map[string][]string{
	"api.openai.com":                    {"openai"},
	"api.anthropic.com":                 {"anthropic"},
	"generativelanguage.googleapis.com": {"google"},
	"api.x.ai":                          {"xai"},
	"api.mistral.ai":                    {"mistral"},
	"api.groq.com":                      {"groq"},
	"api.cerebras.ai":                   {"cerebras"},
	"api.cohere.com":                    {"cohere"},
	"api.cohere.ai":                     {"cohere"},
	"api.deepinfra.com":                 {"deepinfra"},
	"api.together.xyz":                  {"togetherai"},
	"api.perplexity.ai":                 {"perplexity"},
	"aihubmix.com":                      {"aihubmix"},
	"api.venice.ai":                     {"venice"},
	"ai-gateway.vercel.sh":              {"vercel"},
	"gateway.ai.cloudflare.com":         {"cloudflare-ai-gateway"},
	"api.v0.dev":                        {"v0"},
	"gitlab.com":                        {"gitlab"},
	"cloud.gitlab.com":                  {"gitlab"},
	"ai.salad.cloud":                    {"salad-cloud"},
}

// variantSuffixes are stripped, longest first, as a last-resort lookup. They
// cover effort/reasoning labels that resellers append to an otherwise
// catalogued model id (for example gemini-3.8-flash-high).
var variantSuffixes = []string{
	"-non-thinking",
	"-no-thinking",
	"-thinking",
	"-preview",
	"-minimal",
	"-medium",
	"-xhigh",
	"-high",
	"-low",
	" (thinking)",
	" (xhigh)",
	" (high)",
	" (medium)",
	" (low)",
	"(thinking)",
	"(xhigh)",
	"(high)",
	"(medium)",
	"(low)",
}

type candidate struct {
	providerID string
	modelID    string
	model      Model
}

// Index is the lookup structure built from one snapshot. It is immutable after
// construction; a new catalog version builds a new index.
type Index struct {
	providers  map[string]Provider
	order      []string
	exact      map[string]map[string]Model
	lower      map[string]map[string]Model
	normalized map[string]map[string]Model
	global     map[string][]candidate
	apiHosts   map[string][]string
	apiPaths   map[string][]string
	known      map[string]struct{}
}

// NewIndex builds the lookup index for a snapshot.
func NewIndex(snapshot *Snapshot) *Index {
	index := &Index{
		providers:  map[string]Provider{},
		exact:      map[string]map[string]Model{},
		lower:      map[string]map[string]Model{},
		normalized: map[string]map[string]Model{},
		global:     map[string][]candidate{},
		apiHosts:   map[string][]string{},
		apiPaths:   map[string][]string{},
		known:      map[string]struct{}{},
	}
	if snapshot == nil {
		return index
	}
	for providerID, provider := range snapshot.Providers {
		index.providers[providerID] = provider
		index.order = append(index.order, providerID)
		index.known[providerID] = struct{}{}
		exact := make(map[string]Model, len(provider.Models))
		lower := make(map[string]Model, len(provider.Models))
		normalized := make(map[string]Model, len(provider.Models))
		for modelID, model := range provider.Models {
			exact[modelID] = model
			lowerKey := strings.ToLower(modelID)
			if _, exists := lower[lowerKey]; !exists {
				lower[lowerKey] = model
			}
			normKey := NormalizeModelID(modelID)
			if _, exists := normalized[normKey]; !exists {
				normalized[normKey] = model
			}
			index.global[normKey] = append(index.global[normKey], candidate{providerID: providerID, modelID: modelID, model: model})
		}
		index.exact[providerID] = exact
		index.lower[providerID] = lower
		index.normalized[providerID] = normalized
		if host := apiHost(provider.API); host != "" {
			index.apiHosts[host] = append(index.apiHosts[host], providerID)
			index.apiPaths[providerID] = pathSegments(provider.API)
		}
	}
	sort.Strings(index.order)
	return index
}

// NormalizeModelID lowercases a model id and removes the ":free" and "@<digits>"
// suffixes that aggregators append.
func NormalizeModelID(id string) string {
	value := strings.ToLower(strings.TrimSpace(id))
	if strings.HasSuffix(value, ":free") {
		value = strings.TrimSuffix(value, ":free")
	}
	if at := strings.LastIndex(value, "@"); at >= 0 {
		suffix := value[at+1:]
		if suffix != "" && isDigits(suffix) {
			value = value[:at]
		}
	}
	return value
}

func isDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func apiHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return ""
	}
	return strings.ToLower(parsed.Host)
}

func pathSegments(raw string) []string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil
	}
	trimmed := strings.Trim(parsed.Path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

// Lookup resolves a metered model id to a catalog entry. source is the usage
// source dimension (often a base URL), provider is the CLIProxyAPI channel
// name.
func (i *Index) Lookup(source, provider, model string) (Match, bool) {
	if i == nil || len(i.providers) == 0 {
		return Match{}, false
	}
	target := strings.TrimSpace(model)
	if target == "" {
		return Match{}, false
	}
	hints := i.hints(source, provider)
	if match, ok := i.lookupTarget(target, hints, ""); ok {
		return match, true
	}
	if stripped, ok := stripVariantSuffix(target); ok {
		if match, ok := i.lookupTarget(stripped, hints, MatchVariant); ok {
			return match, true
		}
	}
	return Match{}, false
}

// lookupTarget runs the ladder: hinted providers, slash vendor prefix, then the
// global normalized index. forcedKind overrides the reported match kind (used
// by the variant fallback).
func (i *Index) lookupTarget(target string, hints []string, forcedKind string) (Match, bool) {
	for _, providerID := range hints {
		if match, ok := i.lookupProvider(providerID, target); ok {
			return finishMatch(match, MatchHint, forcedKind), true
		}
	}
	if match, ok := i.lookupVendorPrefix(target); ok {
		return finishMatch(match, MatchVendor, forcedKind), true
	}
	if candidates := i.global[NormalizeModelID(target)]; len(candidates) > 0 {
		best := i.pick(candidates, hints)
		return finishMatch(Match{ProviderID: best.providerID, ModelID: best.modelID, Model: best.model}, MatchGlobal, forcedKind), true
	}
	return Match{}, false
}

func finishMatch(match Match, kind, forcedKind string) Match {
	if forcedKind != "" {
		match.Kind = forcedKind
		return match
	}
	match.Kind = kind
	return match
}

func (i *Index) lookupProvider(providerID, target string) (Match, bool) {
	exact, ok := i.exact[providerID]
	if !ok {
		return Match{}, false
	}
	if model, ok := exact[target]; ok {
		return Match{ProviderID: providerID, ModelID: target, Model: model}, true
	}
	lowerKey := strings.ToLower(target)
	if model, ok := i.lower[providerID][lowerKey]; ok {
		return Match{ProviderID: providerID, ModelID: lowerKey, Model: model}, true
	}
	normKey := NormalizeModelID(target)
	if model, ok := i.normalized[providerID][normKey]; ok {
		return Match{ProviderID: providerID, ModelID: normKey, Model: model}, true
	}
	return Match{}, false
}

func (i *Index) lookupVendorPrefix(target string) (Match, bool) {
	slash := strings.Index(target, "/")
	if slash <= 0 || slash == len(target)-1 {
		return Match{}, false
	}
	providerID, ok := vendorPrefixToProvider[strings.ToLower(target[:slash])]
	if !ok {
		return Match{}, false
	}
	if _, exists := i.providers[providerID]; !exists {
		return Match{}, false
	}
	return i.lookupProvider(providerID, target[slash+1:])
}

// hints derives ordered provider candidates from the usage source URL and the
// CLIProxyAPI channel name.
func (i *Index) hints(source, provider string) []string {
	seen := map[string]struct{}{}
	var hints []string
	appendHint := func(id string) {
		if id == "" {
			return
		}
		if _, exists := i.providers[id]; !exists {
			return
		}
		if _, exists := seen[id]; exists {
			return
		}
		seen[id] = struct{}{}
		hints = append(hints, id)
	}
	for _, id := range i.channelHints(provider) {
		appendHint(id)
	}
	for _, id := range i.sourceHints(source) {
		appendHint(id)
	}
	if len(hints) > 8 {
		hints = hints[:8]
	}
	return hints
}

// channelHints tokenizes the channel name and matches contiguous token runs
// against catalog provider ids. Longest runs win so
// "openai-compatible-opencode-go" prefers "opencode-go" over "opencode".
func (i *Index) channelHints(provider string) []string {
	tokens := channelTokens(provider)
	if len(tokens) == 0 {
		return nil
	}
	type hit struct {
		id    string
		words int
	}
	var hits []hit
	for start := 0; start < len(tokens); start++ {
		var joined strings.Builder
		for end := start; end < len(tokens) && end < start+6; end++ {
			if end > start {
				joined.WriteByte('-')
			}
			joined.WriteString(tokens[end])
			value := joined.String()
			if len(value) > 64 {
				break
			}
			if _, exists := i.known[value]; exists {
				hits = append(hits, hit{id: value, words: end - start + 1})
			}
		}
	}
	sort.SliceStable(hits, func(a, b int) bool {
		if hits[a].words != hits[b].words {
			return hits[a].words > hits[b].words
		}
		if len(hits[a].id) != len(hits[b].id) {
			return len(hits[a].id) > len(hits[b].id)
		}
		return hits[a].id < hits[b].id
	})
	ordered := make([]string, 0, len(hits))
	for _, item := range hits {
		ordered = append(ordered, item.id)
	}
	return ordered
}

// channelTokens lowercases a channel name, removes the openai-compatible
// prefix, and splits it into alphanumeric tokens.
func channelTokens(provider string) []string {
	value := strings.ToLower(strings.TrimSpace(provider))
	if value == "" {
		return nil
	}
	for {
		trimmed := value
		for _, prefix := range []string{"openai-compatible-", "openai_compatible_", "openai compatible "} {
			if strings.HasPrefix(trimmed, prefix) {
				trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
			}
		}
		if trimmed == value {
			break
		}
		value = trimmed
	}
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	})
	out := fields[:0]
	for _, field := range fields {
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

// sourceHints maps a usage source URL to catalog providers: exact host table
// first, then catalog api hosts, then the known cloud host suffixes.
func (i *Index) sourceHints(source string) []string {
	parsed, err := url.Parse(strings.TrimSpace(source))
	if err != nil || parsed.Host == "" {
		return nil
	}
	if parsed.Scheme != "" && parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil
	}
	host := strings.ToLower(parsed.Host)
	var hints []string
	hints = append(hints, exactHostProviders[host]...)
	for _, id := range i.apiHosts[host] {
		hints = append(hints, id)
	}
	hints = append(hints, suffixHostProviders(host)...)
	if len(hints) == 0 {
		return nil
	}
	sourcePath := pathSegments(source)
	sort.SliceStable(hints, func(a, b int) bool {
		overlapA := pathOverlap(sourcePath, i.apiPaths[hints[a]])
		overlapB := pathOverlap(sourcePath, i.apiPaths[hints[b]])
		if overlapA != overlapB {
			return overlapA > overlapB
		}
		if len(hints[a]) != len(hints[b]) {
			return len(hints[a]) > len(hints[b])
		}
		return hints[a] < hints[b]
	})
	return hints
}

func pathOverlap(source, provider []string) int {
	count := 0
	for count < len(source) && count < len(provider) {
		if !strings.EqualFold(source[count], provider[count]) {
			break
		}
		count++
	}
	return count
}

// suffixHostProviders mirrors the host suffix rules used for cloud endpoints
// whose base URL is not listed verbatim in the catalog.
func suffixHostProviders(host string) []string {
	switch {
	case host == "aiplatform.googleapis.com",
		strings.HasSuffix(host, ".aiplatform.googleapis.com"),
		strings.HasSuffix(host, "-aiplatform.googleapis.com"):
		return []string{"google-vertex", "google-vertex-anthropic"}
	case strings.HasSuffix(host, ".openai.azure.com"):
		return []string{"azure"}
	case strings.HasSuffix(host, ".cognitiveservices.azure.com"):
		return []string{"azure-cognitive-services"}
	case strings.HasPrefix(host, "bedrock-runtime.") && strings.HasSuffix(host, ".amazonaws.com"):
		return []string{"amazon-bedrock"}
	case strings.HasSuffix(host, ".aihubmix.com"):
		return []string{"aihubmix"}
	case host == "ai.salad.cloud", strings.HasSuffix(host, ".salad.cloud"):
		return []string{"salad-cloud"}
	case strings.HasSuffix(host, ".gitlab.com"):
		return []string{"gitlab"}
	case strings.HasSuffix(host, ".ml.cloud.ibm.com"):
		return []string{"watsonx"}
	}
	return nil
}

// stripVariantSuffix removes one trailing effort/reasoning label.
func stripVariantSuffix(target string) (string, bool) {
	lowered := strings.ToLower(target)
	for _, suffix := range variantSuffixes {
		if len(lowered) <= len(suffix) || !strings.HasSuffix(lowered, suffix) {
			continue
		}
		stripped := strings.TrimSpace(target[:len(target)-len(suffix)])
		if stripped == "" {
			continue
		}
		return stripped, true
	}
	return "", false
}

// pick chooses the best candidate: hinted providers first, then records with a
// non-zero list price (so an unknown reseller's $0 does not win), then the
// first-party ranking, provider prefix affinity and finally the provider id
// for determinism.
func (i *Index) pick(candidates []candidate, hints []string) candidate {
	hintPosition := make(map[string]int, len(hints))
	for position, id := range hints {
		if _, exists := hintPosition[id]; !exists {
			hintPosition[id] = position
		}
	}
	best := candidates[0]
	for _, current := range candidates[1:] {
		if i.compare(current, best, hintPosition) < 0 {
			best = current
		}
	}
	return best
}

func (i *Index) compare(left, right candidate, hintPosition map[string]int) int {
	if value := hintRank(left.providerID, hintPosition) - hintRank(right.providerID, hintPosition); value != 0 {
		return value
	}
	if value := zeroPriceRank(left.model) - zeroPriceRank(right.model); value != 0 {
		return value
	}
	if value := priorityRank(left.providerID) - priorityRank(right.providerID); value != 0 {
		return value
	}
	if value := prefixAffinity(left) - prefixAffinity(right); value != 0 {
		return value
	}
	return strings.Compare(left.providerID, right.providerID)
}

func hintRank(providerID string, hintPosition map[string]int) int {
	if position, exists := hintPosition[providerID]; exists {
		return position
	}
	return 1 << 20
}

func zeroPriceRank(model Model) int {
	if model.Cost.Priced() && !model.Cost.Free() {
		return 0
	}
	return 1
}

func priorityRank(providerID string) int {
	for position, id := range firstPartyPriority {
		if id == providerID {
			return position
		}
	}
	return len(firstPartyPriority) + 1
}

func prefixAffinity(item candidate) int {
	provider := strings.ToLower(item.providerID)
	if provider == "" {
		return 1
	}
	if strings.HasPrefix(strings.ToLower(item.modelID), provider) {
		return 0
	}
	if family := strings.ToLower(item.model.Family); family != "" && strings.HasPrefix(family, provider) {
		return 0
	}
	return 1
}

// ProviderIDs returns the catalog provider ids in stable order.
func (i *Index) ProviderIDs() []string {
	if i == nil {
		return nil
	}
	return append([]string(nil), i.order...)
}

// HasProvider reports whether the catalog knows the provider id.
func (i *Index) HasProvider(id string) bool {
	if i == nil {
		return false
	}
	_, exists := i.providers[id]
	return exists
}
