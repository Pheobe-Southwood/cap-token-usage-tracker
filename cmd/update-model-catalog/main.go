// Command update-model-catalog regenerates the embedded models.dev snapshot.
//
// Usage:
//
//	go run ./cmd/update-model-catalog
//	go run ./cmd/update-model-catalog -input /tmp/models_dev_api.json
//	go run ./cmd/update-model-catalog -url https://models.dev/api.json -out internal/plugin/modelcatalog/baseline.json
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/AITNR/cap-token-usage-tracker/internal/plugin/modelcatalog"
)

func main() {
	defaultOutput := filepath.Join("internal", "plugin", "modelcatalog", "baseline.json")
	urlFlag := flag.String("url", modelcatalog.DefaultURL, "upstream catalog URL")
	inputFlag := flag.String("input", "", "read a local payload instead of downloading")
	outputFlag := flag.String("out", defaultOutput, "output snapshot path")
	flag.Parse()

	raw, err := readPayload(*inputFlag, *urlFlag)
	if err != nil {
		fail(err)
	}
	trimmed, snapshot, err := modelcatalog.Trim(raw, time.Now().UTC())
	if err != nil {
		fail(err)
	}
	if !snapshot.Valid() {
		fail(fmt.Errorf("trimmed catalog has no providers or models"))
	}
	if err := os.MkdirAll(filepath.Dir(*outputFlag), 0o755); err != nil {
		fail(err)
	}
	if err := os.WriteFile(*outputFlag, append(trimmed, '\n'), 0o644); err != nil {
		fail(err)
	}
	stats := snapshot.ComputeStats()
	fmt.Printf("providers: %d\nmodels: %d\npriced: %d\nfree: %d\nbytes: %d\noutput: %s\n",
		stats.Providers, stats.Models, stats.Priced, stats.Free, len(trimmed)+1, *outputFlag)
}

func readPayload(input, endpoint string) ([]byte, error) {
	if input != "" {
		raw, err := os.ReadFile(input)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", input, err)
		}
		return raw, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s: HTTP %d", endpoint, response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, int64(modelcatalog.MaxPayloadBytes)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > modelcatalog.MaxPayloadBytes {
		return nil, fmt.Errorf("payload exceeds %d bytes", modelcatalog.MaxPayloadBytes)
	}
	return raw, nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
