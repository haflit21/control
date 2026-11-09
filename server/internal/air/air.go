// Package air fetches outdoor air quality from Open-Meteo (free, no key) and
// turns it into the ATMO-style six-level index the interface shows.
package air

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"ahomelab.io/control/internal/model"
)

// Fetcher returns the current air quality for a place.
type Fetcher interface {
	Fetch(ctx context.Context, lat, lon float64) (model.Air, error)
}

// OpenMeteo talks to air-quality-api.open-meteo.com.
type OpenMeteo struct {
	HTTP *http.Client
	Base string // for tests; default https://air-quality-api.open-meteo.com
}

// Labels of the six ATMO levels (index 1–6).
var Labels = [7]string{"", "Bon", "Moyen", "Dégradé", "Mauvais", "Très mauvais", "Extrêmement mauvais"}

// pollutants in display order; the key is what the UI knows.
var pollutants = []struct{ key, cur, aqi string }{
	{"pm25", "pm2_5", "european_aqi_pm2_5"},
	{"pm10", "pm10", "european_aqi_pm10"},
	{"no2", "nitrogen_dioxide", "european_aqi_nitrogen_dioxide"},
	{"o3", "ozone", "european_aqi_ozone"},
	{"so2", "sulphur_dioxide", "european_aqi_sulphur_dioxide"},
}

var pollens = []struct{ key, cur string }{
	{"grass", "grass_pollen"}, {"birch", "birch_pollen"}, {"alder", "alder_pollen"},
	{"mugwort", "mugwort_pollen"}, {"olive", "olive_pollen"}, {"ragweed", "ragweed_pollen"},
}

type response struct {
	Current map[string]any `json:"current"`
	Hourly  struct {
		Time        []string  `json:"time"`
		EuropeanAQI []float64 `json:"european_aqi"`
	} `json:"hourly"`
}

func (o *OpenMeteo) Fetch(ctx context.Context, lat, lon float64) (model.Air, error) {
	base := o.Base
	if base == "" {
		base = "https://air-quality-api.open-meteo.com"
	}
	cur := []string{"european_aqi"}
	for _, p := range pollutants {
		cur = append(cur, p.cur, p.aqi)
	}
	for _, p := range pollens {
		cur = append(cur, p.cur)
	}
	q := url.Values{}
	q.Set("latitude", fmt.Sprintf("%.4f", lat))
	q.Set("longitude", fmt.Sprintf("%.4f", lon))
	q.Set("current", join(cur))
	q.Set("hourly", "european_aqi")
	q.Set("forecast_days", "2")
	q.Set("timezone", "auto")
	req, err := http.NewRequestWithContext(ctx, "GET", base+"/v1/air-quality?"+q.Encode(), nil)
	if err != nil {
		return model.Air{}, err
	}
	hc := o.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return model.Air{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return model.Air{}, fmt.Errorf("open-meteo : HTTP %d", resp.StatusCode)
	}
	var r response
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil {
		return model.Air{}, err
	}
	return FromOpenMeteo(r.Current, r.Hourly.Time, r.Hourly.EuropeanAQI, time.Now()), nil
}

// FromOpenMeteo normalises a "current" block (and tomorrow's hourly EAQI) into model.Air.
func FromOpenMeteo(cur map[string]any, hourlyTime []string, hourlyAQI []float64, now time.Time) model.Air {
	a := model.Air{Available: true, Source: "Open-Meteo · CAMS", UpdatedAt: now, Pollutants: []model.Pollutant{}, Pollens: []model.Pollen{}}
	if v, ok := num(cur["european_aqi"]); ok {
		a.Index = Level(v)
		a.Label = Labels[a.Index]
	}
	best := 0
	for _, p := range pollutants {
		val, okv := num(cur[p.cur])
		aqi, oka := num(cur[p.aqi])
		if !okv && !oka {
			continue
		}
		lvl := 0
		if oka {
			lvl = Level(aqi)
		}
		a.Pollutants = append(a.Pollutants, model.Pollutant{Key: p.key, Index: lvl, Value: round1(val)})
		if oka && int(aqi) > best {
			best = int(aqi)
			a.Dominant = p.key
		}
	}
	for _, p := range pollens {
		if v, ok := num(cur[p.cur]); ok {
			a.Pollens = append(a.Pollens, model.Pollen{Key: p.key, Level: PollenLevel(v), Value: round1(v)})
		}
	}
	// tomorrow: worst hourly EAQI of the next calendar day
	tomorrow := now.AddDate(0, 0, 1).Format("2006-01-02")
	worst := 0.0
	for i, t := range hourlyTime {
		if i < len(hourlyAQI) && len(t) >= 10 && t[:10] == tomorrow && hourlyAQI[i] > worst {
			worst = hourlyAQI[i]
		}
	}
	if worst > 0 {
		a.Tomorrow = Level(worst)
	}
	if a.Index == 0 {
		a.Available = false
	}
	return a
}

// Level maps the European AQI (0–100+) to the six ATMO levels.
func Level(eaqi float64) int {
	switch {
	case eaqi <= 20:
		return 1
	case eaqi <= 40:
		return 2
	case eaqi <= 60:
		return 3
	case eaqi <= 80:
		return 4
	case eaqi <= 100:
		return 5
	default:
		return 6
	}
}

// PollenLevel maps grains/m³ to 0–6 (none … very high), a coarse RNSA-like scale.
func PollenLevel(g float64) int {
	switch {
	case g <= 0:
		return 0
	case g < 10:
		return 1
	case g < 25:
		return 2
	case g < 50:
		return 3
	case g < 100:
		return 4
	case g < 300:
		return 5
	default:
		return 6
	}
}

func num(v any) (float64, bool) {
	f, ok := v.(float64)
	return f, ok
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }

func join(s []string) string {
	out := ""
	for i, x := range s {
		if i > 0 {
			out += ","
		}
		out += x
	}
	return out
}
