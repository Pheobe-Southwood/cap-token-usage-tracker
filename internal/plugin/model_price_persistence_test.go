package plugin

import (
	"errors"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func TestModelPriceOverridesPersistAndSurviveReset(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	price := priceOf(0.25, 1.5, mathNaN(), mathNaN())
	overrides, err := store.SetModelPriceOverride("Beam", &price)
	if err != nil {
		t.Fatal(err)
	}
	if len(overrides) != 1 || priceValue(overrides["Beam"].Input) != 0.25 || priceValue(overrides["Beam"].Output) != 1.5 {
		t.Fatalf("overrides = %+v", overrides)
	}
	if err := store.Record(normalizedUsage{Dimensions: Dimensions{Model: "Beam"}, RequestedAt: nowUTC(), Counters: Counters{Requests: 1, TotalTokens: 5}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	// A reopened database keeps the manual prices.
	store, err = openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.ModelPriceOverrides()
	if err != nil || len(stored) != 1 || priceValue(stored["Beam"].Output) != 1.5 {
		t.Fatalf("reloaded overrides = %+v, %v", stored, err)
	}

	// Manual prices are configuration: a statistics reset keeps them.
	if err := store.Reset(); err != nil {
		t.Fatal(err)
	}
	stored, err = store.ModelPriceOverrides()
	if err != nil || len(stored) != 1 {
		t.Fatalf("overrides after reset = %+v, %v", stored, err)
	}
	stats, err := store.Query("24h")
	if err != nil || stats.Summary.Requests != 0 {
		t.Fatalf("usage data survived reset: %+v, %v", stats.Summary, err)
	}

	// Clearing with nil removes the entry, including the persisted key.
	if overrides, err = store.SetModelPriceOverride("Beam", nil); err != nil || len(overrides) != 0 {
		t.Fatalf("cleared overrides = %+v, %v", overrides, err)
	}
	if stored, err = store.ModelPriceOverrides(); err != nil || len(stored) != 0 {
		t.Fatalf("stored overrides after clear = %+v, %v", stored, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := bolt.Open(config.DataPath, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.View(func(tx *bolt.Tx) error {
		if raw := tx.Bucket(metaBucket).Get(modelPriceOverridesKey); len(raw) != 0 {
			return errors.New("empty override table left a metadata key behind")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestModelPriceOverrideValidation(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cases := []struct {
		name  string
		model string
		price ModelPrice
		code  int
	}{
		{name: "empty model", model: "  ", price: priceOf(1, 1, mathNaN(), mathNaN()), code: 400},
		{name: "negative price", model: "Beam", price: priceOf(-1, mathNaN(), mathNaN(), mathNaN()), code: 400},
		{name: "oversized price", model: "Beam", price: priceOf(maxModelPriceValue*2, mathNaN(), mathNaN(), mathNaN()), code: 400},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			price := testCase.price
			if _, err := store.SetModelPriceOverride(testCase.model, &price); errorHTTPStatus(err) != testCase.code {
				t.Fatalf("error = %v, want status %d", err, testCase.code)
			}
		})
	}
	// An empty price clears instead of failing.
	price := ModelPrice{}
	if overrides, err := store.SetModelPriceOverride("Beam", &price); err != nil || len(overrides) != 0 {
		t.Fatalf("empty price = %+v, %v", overrides, err)
	}
	// The table limit is enforced before touching storage.
	raw := make(map[string]ModelPrice, maxModelPriceOverrides+1)
	for index := 0; index <= maxModelPriceOverrides; index++ {
		raw["model-"+itoa(index)] = priceOf(1, 1, mathNaN(), mathNaN())
	}
	if _, err := normalizeModelPriceOverrides(raw); err == nil {
		t.Fatalf("oversized override table accepted")
	}
}

func TestCorruptModelPriceOverridesRejectedOnOpen(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	price := priceOf(1, 1, mathNaN(), mathNaN())
	if _, err := store.SetModelPriceOverride("Beam", &price); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := bolt.Open(config.DataPath, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(metaBucket).Put(modelPriceOverridesKey, []byte("{not json"))
	}); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := openStore(config); err == nil {
		t.Fatalf("corrupt override table must fail the open")
	}
}

func TestModelPriceOverridesSurviveBackupRestore(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	price := priceOf(2, 4, mathNaN(), mathNaN())
	if _, err := store.SetModelPriceOverride("Beam", &price); err != nil {
		t.Fatal(err)
	}
	backup, err := store.Backup()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetModelPriceOverride("Beam", nil); err != nil {
		t.Fatal(err)
	}
	if err := store.RestoreBackup(backup); err != nil {
		t.Fatal(err)
	}
	overrides, err := store.ModelPriceOverrides()
	if err != nil || len(overrides) != 1 || priceValue(overrides["Beam"].Input) != 2 {
		t.Fatalf("overrides after restore = %+v, %v", overrides, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestModelPriceSchedulePersistsAndSurvivesRateOnlySave(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	schedule := ModelPrice{
		Input:     timeRate(0.2),
		Output:    timeRate(0.8),
		TimeZone:  "Asia/Shanghai",
		TimeTiers: []TimePriceTier{weekdayTier("night", "23:00", "08:00", 1, 7)},
	}
	if _, err := store.SetModelPriceOverride("deepseek-v4.1-flash", &schedule); err != nil {
		t.Fatal(err)
	}
	// A stored schedule survives a reopen with its weekday list intact.
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	stored, err := store.ModelPriceOverrides()
	if err != nil || len(stored) != 1 {
		t.Fatalf("reloaded overrides = %+v, %v", stored, err)
	}
	price := stored["deepseek-v4.1-flash"]
	if price.TimeZone != "Asia/Shanghai" || len(price.TimeTiers) != 1 || len(price.TimeTiers[0].Days) != 2 {
		t.Fatalf("schedule lost across reopen: %+v", price)
	}
	if _, err := normalizeModelPrice("deepseek-v4.1-flash", price); err != nil {
		t.Fatalf("reloaded schedule is invalid: %v", err)
	}
}

func TestLegacyOverrideWithoutScheduleStillLoads(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	// Write the pre-v2.6.0 JSON shape directly, as an upgrade would find it.
	db, err := bolt.Open(config.DataPath, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(metaBucket).Put(modelPriceOverridesKey, []byte(`{"gemini-4-argon":{"input":4,"output":20,"cache_read":0.2}}`))
	}); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	stored, err := store.ModelPriceOverrides()
	if err != nil || len(stored) != 1 || priceValue(stored["gemini-4-argon"].Output) != 20 {
		t.Fatalf("legacy overrides = %+v, %v", stored, err)
	}
	if stored["gemini-4-argon"].TimeTiers != nil {
		t.Fatalf("legacy override gained a schedule: %+v", stored["gemini-4-argon"])
	}
}
