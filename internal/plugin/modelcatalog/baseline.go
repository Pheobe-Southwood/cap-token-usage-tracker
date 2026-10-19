package modelcatalog

import (
	_ "embed"
	"fmt"
	"sync"
)

// baselineJSON is the trimmed models.dev snapshot shipped inside the plugin.
// It is regenerated with:
//
//	go run ./cmd/update-model-catalog
//
// The embedded snapshot guarantees that prices render on a fresh install even
// when the host has no outbound network access.
//
//go:embed baseline.json
var baselineJSON []byte

var (
	embeddedOnce     sync.Once
	embeddedSnapshot *Snapshot
	embeddedErr      error
)

// Embedded returns the parsed built-in snapshot. Parsing happens once per
// plugin instance.
func Embedded() (*Snapshot, error) {
	embeddedOnce.Do(func() {
		embeddedSnapshot, embeddedErr = Parse(baselineJSON)
		if embeddedErr != nil {
			embeddedErr = fmt.Errorf("parse embedded model catalog: %w", embeddedErr)
		}
	})
	return embeddedSnapshot, embeddedErr
}
