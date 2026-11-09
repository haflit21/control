package air

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLevels(t *testing.T) {
	for eaqi, want := range map[float64]int{0: 1, 20: 1, 21: 2, 40: 2, 55: 3, 61: 4, 99: 5, 140: 6} {
		if got := Level(eaqi); got != want {
			t.Errorf("Level(%v) = %d, want %d", eaqi, got, want)
		}
	}
	if PollenLevel(0) != 0 || PollenLevel(5) != 1 || PollenLevel(120) != 5 || PollenLevel(400) != 6 {
		t.Error("PollenLevel")
	}
}

func TestFetchOpenMeteo(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("latitude") != "45.6970" {
			t.Errorf("latitude = %q", r.URL.Query().Get("latitude"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"current": map[string]any{"european_aqi": 66.0, "pm2_5": 38.0, "european_aqi_pm2_5": 66.0, "ozone": 40.0, "european_aqi_ozone": 20.0, "grass_pollen": 30.0},
			"hourly":  map[string]any{"time": []string{"2026-11-10T08:00", "2026-11-10T14:00"}, "european_aqi": []float64{30, 52}},
		})
	}))
	defer srv.Close()
	a, err := (&OpenMeteo{Base: srv.URL}).Fetch(context.Background(), 45.697, 4.886)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Available || a.Index != 4 || a.Label != "Mauvais" || a.Dominant != "pm25" {
		t.Fatalf("air = %+v", a)
	}
	if len(a.Pollutants) != 2 || a.Pollutants[0].Key != "pm25" || a.Pollutants[0].Index != 4 || a.Pollutants[0].Value != 38 {
		t.Fatalf("pollutants = %+v", a.Pollutants)
	}
	if len(a.Pollens) != 1 || a.Pollens[0].Level != 3 {
		t.Fatalf("pollens = %+v", a.Pollens)
	}
	// tomorrow is computed from time.Now in Fetch; check the pure function with a fixed clock
	b := FromOpenMeteo(map[string]any{"european_aqi": 10.0}, []string{"2026-11-10T14:00"}, []float64{52}, now)
	if b.Index != 1 || b.Tomorrow != 3 {
		t.Fatalf("tomorrow = %+v", b)
	}
}
