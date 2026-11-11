// Package demo is a Home Assistant stand-in: it invents a plausible state
// for every entity control.yaml refers to and makes it drift a little, so
// the interface can be developed, screenshotted and tested without the
// house (CONTROL_DEMO=1).
package demo

import (
	"ahomelab.io/control/internal/agenda"
	"ahomelab.io/control/internal/air"
	"ahomelab.io/control/internal/model"
	"context"
	"fmt"
	"io"
	"math"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ahomelab.io/control/internal/config"
	"ahomelab.io/control/internal/ha"
)

type Source struct {
	*ha.Store
	cfg  *config.Config
	rnd  *rand.Rand
	tick int

	// delayed answers land in the order the calls were made
	seq     atomic.Uint64
	seqMu   sync.Mutex
	applied map[string]uint64
}

// later applies changed after the demo's answer delay, unless a newer call
// already changed the same entity.
func (s *Source) later(ctx context.Context, changed []ha.Entity) {
	seq := s.seq.Add(1)
	go func() {
		select {
		case <-time.After(350 * time.Millisecond):
		case <-ctx.Done():
		}
		s.seqMu.Lock()
		defer s.seqMu.Unlock()
		if s.applied == nil {
			s.applied = map[string]uint64{}
		}
		keep := []ha.Entity{}
		for _, e := range changed {
			if s.applied[e.EntityID] < seq {
				s.applied[e.EntityID] = seq
				keep = append(keep, e)
			}
		}
		s.Set(keep...)
	}()
}

func New(cfg *config.Config) *Source {
	s := &Source{Store: ha.NewStore(), cfg: cfg, rnd: rand.New(rand.NewSource(42))}
	s.seed(time.Now())
	s.SetConnected(true)
	return s
}

func (s *Source) Connected() bool { return true }

// Run drifts temperatures every 20 s and flips a light every 45 s.
func (s *Source) Run(ctx context.Context) {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			s.tick++
			s.drift(now)
		}
	}
}

func ent(id, state string, attrs map[string]any, at time.Time) ha.Entity {
	if attrs == nil {
		attrs = map[string]any{}
	}
	return ha.Entity{EntityID: id, State: state, Attributes: attrs, LastChanged: at, LastUpdated: at}
}

type roomDemo struct {
	temp, target float64
	mode         string // schedule | manual | off
	heating      bool
	humidity     float64
	window       bool
	windowAgo    time.Duration
}

var roomsDemo = map[string]roomDemo{
	"salon":   {temp: 19.5, target: 21, mode: "schedule", heating: true, humidity: 54},
	"cuisine": {temp: 18.5, target: 21, mode: "schedule", heating: true, humidity: 61, window: true, windowAgo: 20 * time.Minute},
	"entree":  {temp: 20, target: 20, mode: "schedule", heating: false, humidity: 50},
	"bureau":  {temp: 18.2, target: 0, mode: "off", heating: false, humidity: 48},
	"chambre": {temp: 19.2, target: 0, mode: "off", heating: false, humidity: 50},
	"bebe":    {temp: 19.0, target: 0, mode: "off", heating: false, humidity: 52},
}

func (s *Source) seed(now time.Time) {
	var out []ha.Entity
	add := func(e ha.Entity) { out = append(out, e) }
	for _, r := range s.cfg.Rooms {
		rd := roomsDemo[r.ID]
		for _, d := range r.Devices {
			switch d.Kind {
			case "heating":
				state, action := "heat", "idle"
				if rd.mode == "off" {
					state = "off"
					action = "off"
				}
				if rd.heating {
					action = "heating"
				}
				attrs := map[string]any{"current_temperature": rd.temp, "current_humidity": rd.humidity, "hvac_action": action, "hvac_modes": []any{"heat", "off"}, "friendly_name": r.Name}
				if rd.mode != "off" {
					attrs["temperature"] = rd.target
				}
				add(ent(d.Entities["climate"], state, attrs, now.Add(-3*time.Minute)))
				if e := d.Entities["temperature"]; e != "" {
					add(ent(e, fmt.Sprintf("%.1f", rd.temp), map[string]any{"unit_of_measurement": "°C"}, now.Add(-2*time.Minute)))
				}
				if e := d.Entities["humidity"]; e != "" {
					add(ent(e, fmt.Sprintf("%.0f", rd.humidity), map[string]any{"unit_of_measurement": "%"}, now.Add(-2*time.Minute)))
				}
				if e := d.Entities["heating_power"]; e != "" {
					p := "0"
					if rd.heating {
						p = "62"
					}
					add(ent(e, p, map[string]any{"unit_of_measurement": "%"}, now.Add(-2*time.Minute)))
				}
				if e := d.Entities["mold_risk"]; e != "" {
					add(ent(e, "off", nil, now.Add(-time.Hour)))
				}
				if e := d.Entities["battery"]; e != "" {
					b := "off"
					if r.ID == "salon" && d.ID == "tete" {
						b = "on" // the known low battery
					}
					add(ent(e, b, map[string]any{"device_class": "battery"}, now.Add(-6*time.Hour)))
				}
				if e := d.Entities["zone_mode"]; e != "" {
					zm := map[string]string{"schedule": "Smart schedule", "manual": "Manual", "off": "Off"}[rd.mode]
					attrs := map[string]any{}
					if rd.mode == "manual" {
						attrs["overlay_end"] = now.Add(time.Hour).Format(time.RFC3339)
					}
					add(ent(e, zm, attrs, now.Add(-10*time.Minute)))
				}
			case "ac":
				add(ent(d.Entities["climate"], "off", map[string]any{"current_temperature": rd.temp, "temperature": 24.0, "hvac_modes": []any{"off", "cool", "heat", "fan_only", "dry", "auto"}}, now.Add(-time.Hour)))
				for _, role := range []string{"eco", "boost"} {
					if e := d.Entities[role]; e != "" {
						add(ent(e, "off", nil, now.Add(-time.Hour)))
					}
				}
				if e := d.Entities["power"]; e != "" {
					add(ent(e, "0", map[string]any{"unit_of_measurement": "W"}, now.Add(-time.Minute)))
				}
			case "light", "light_group":
				on := d.ID == "hue1" || d.ID == "hue2" || d.ID == "canape" || d.ID == "sofa1" || d.ID == "sofa2"
				state := "off"
				attrs := map[string]any{"supported_color_modes": []any{"color_temp", "hs"}, "friendly_name": d.Name}
				if on {
					state = "on"
					attrs["brightness"] = 153.0
					attrs["color_temp_kelvin"] = 2700.0
					switch d.ID {
					case "hue1":
						attrs["hs_color"] = []any{30.0, 70.0} // amber
					case "hue2":
						attrs["hs_color"] = []any{205.0, 85.0} // blue
					case "canape", "sofa1", "sofa2":
						attrs["hs_color"] = []any{290.0, 55.0} // violet
					}
				}
				add(ent(d.Entities["light"], state, attrs, now.Add(-36*time.Minute)))
			case "cover":
				if d.ID == "store" {
					add(ent(d.Entities["cover"], "unavailable", nil, now.Add(-72*time.Hour)))
				} else {
					add(ent(d.Entities["cover"], "open", map[string]any{"current_position": 60.0}, now.Add(-3*time.Hour)))
				}
			case "plug":
				add(ent(d.Entities["switch"], "on", nil, now.Add(-2*time.Hour)))
				if e := d.Entities["power"]; e != "" {
					add(ent(e, "86", map[string]any{"unit_of_measurement": "W"}, now.Add(-30*time.Second)))
				}
			case "opening":
				if r.ID == "salon" || r.ID == "bureau" {
					add(ent(d.Entities["contact"], "unavailable", nil, now.Add(-30*time.Hour)))
				} else {
					state := "off"
					at := now.Add(-5 * time.Hour)
					if rd.window {
						state = "on"
						at = now.Add(-rd.windowAgo)
					}
					add(ent(d.Entities["contact"], state, map[string]any{"device_class": "window"}, at))
				}
			case "motion":
				add(ent(d.Entities["motion"], "off", nil, now.Add(-12*time.Minute)))
			case "camera":
				add(ent(d.Entities["camera"], "recording", map[string]any{"friendly_name": d.Name}, now.Add(-8*time.Second)))
			case "media":
				attrs := map[string]any{"volume_level": 0.35, "media_title": "France Inter", "media_channel": "Radio", "media_content_type": "music"}
				if len(s.cfg.Media.Radios) > 0 {
					attrs["media_title"], attrs["media_content_id"] = s.cfg.Media.Radios[0].Name, s.cfg.Media.Radios[0].URL
				}
				add(ent(d.Entities["media_player"], "playing", attrs, now.Add(-80*time.Minute)))
				if e := d.Entities["music"]; e != "" {
					add(ent(e, "idle", map[string]any{"mass_player_type": "player", "supported_features": float64(massFeatures), "volume_level": 0.35}, now.Add(-80*time.Minute)))
				}
			case "smoke":
				add(ent(d.Entities["smoke"], "off", nil, now.Add(-200*time.Hour)))
				if e := d.Entities["co"]; e != "" {
					add(ent(e, "off", nil, now.Add(-200*time.Hour)))
				}
			}
		}
	}
	g := s.cfg.Globals
	add(ent(g.Average.Temperature, "19.8", map[string]any{"unit_of_measurement": "°C"}, now.Add(-time.Minute)))
	add(ent(g.Average.Humidity, "52", map[string]any{"unit_of_measurement": "%"}, now.Add(-time.Minute)))
	for i, p := range g.Presence {
		state := "home"
		if i > 0 {
			state = "not_home"
		}
		add(ent(p.Entity, state, map[string]any{"friendly_name": p.Name}, now.Add(-3*time.Hour)))
	}
	add(ent(g.Heating.Presence, "HOME", nil, now.Add(-4*time.Hour)))
	add(ent(g.Heating.Quota, "84", nil, now.Add(-time.Minute)))
	add(ent(g.Alarm.Armed, "on", nil, now.Add(-2*time.Hour)))
	add(ent(g.Alarm.Acked, "off", nil, now.Add(-2*time.Hour)))
	add(ent(g.Alarm.Unifi, "armed_away", nil, now.Add(-2*time.Hour)))
	add(ent(g.Cameras.Private, "off", nil, now.Add(-2*time.Hour)))
	add(ent(g.Sun, sunState(now), map[string]any{"next_rising": nextRise(now).Format(time.RFC3339), "next_setting": nextSet(now).Format(time.RFC3339)}, now.Add(-time.Minute)))
	add(ent(g.Weather, "cloudy", map[string]any{"temperature": 9.0, "apparent_temperature": 6.0, "humidity": 82.0, "pressure": 1008.0, "wind_speed": 22.0, "wind_bearing": 225.0, "wind_gust_speed": 38.0, "cloud_coverage": 90.0, "visibility": 8.0, "dew_point": 6.0, "uv_index": 0.0, "friendly_name": "Vénissieux", "attribution": "Données Météo-France"}, now.Add(-4*time.Minute)))
	s.Replace(out)
	s.forecasts(now)
}

func (s *Source) forecasts(now time.Time) {
	conds := []string{"cloudy", "cloudy", "rainy", "rainy", "partlycloudy", "clear-night", "clear-night", "fog", "fog", "clear-night", "clear-night", "partlycloudy", "sunny", "sunny", "partlycloudy", "cloudy", "cloudy", "rainy", "rainy", "cloudy", "clear-night", "clear-night", "clear-night", "clear-night"}
	temps := []float64{9, 8, 8, 7, 7, 6, 6, 5, 5, 4, 4, 4, 6, 8, 10, 11, 11, 10, 9, 8, 7, 7, 6, 6}
	pops := []float64{10, 20, 60, 70, 20, 0, 0, 0, 0, 0, 0, 10, 0, 0, 0, 10, 20, 60, 50, 20, 0, 0, 0, 0}
	start := now.Truncate(time.Hour)
	var hourly []map[string]any
	for i := 0; i < 24; i++ {
		hourly = append(hourly, map[string]any{"datetime": start.Add(time.Duration(i) * time.Hour).Format(time.RFC3339), "condition": conds[i], "temperature": temps[i], "precipitation_probability": pops[i], "precipitation": map[bool]float64{true: 0.6, false: 0}[pops[i] >= 60], "wind_speed": 20.0})
	}
	days := []struct {
		c      string
		hi, lo float64
		pop    float64
	}{{"partlycloudy", 12, 5, 20}, {"rainy", 11, 7, 80}, {"cloudy", 12, 5, 30}, {"sunny", 15, 3, 0}, {"snowy", 4, -1, 60}, {"partlycloudy", 9, 2, 10}, {"sunny", 11, 1, 0}}
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var daily []map[string]any
	for i, d := range days {
		daily = append(daily, map[string]any{"datetime": day.AddDate(0, 0, i).Format(time.RFC3339), "condition": d.c, "temperature": d.hi, "templow": d.lo, "precipitation_probability": d.pop, "precipitation": d.pop / 20, "wind_speed": 25.0, "wind_bearing": 200.0, "humidity": 80.0, "uv_index": 1.0})
	}
	s.SetForecast(s.cfg.Globals.Weather, "hourly", hourly)
	s.SetForecast(s.cfg.Globals.Weather, "daily", daily)
}

// drift nudges a few values so the SSE stream shows life.
func (s *Source) drift(now time.Time) {
	st := s.Snapshot()
	var changed []ha.Entity
	for _, r := range s.cfg.Rooms {
		for _, d := range r.Devices {
			if d.Kind == "heating" {
				e, ok := st[d.Entities["climate"]]
				if !ok || e.State == "unavailable" {
					continue
				}
				if t, ok := e.Attributes["current_temperature"].(float64); ok {
					t = math.Round((t+(s.rnd.Float64()-0.5)*0.2)*10) / 10
					e.Attributes["current_temperature"] = t
					e.LastUpdated = now
					changed = append(changed, e)
					if te := d.Entities["temperature"]; te != "" {
						x := st[te]
						x.State = fmt.Sprintf("%.1f", t)
						x.LastUpdated = now
						changed = append(changed, x)
					}
				}
			}
		}
	}
	// every ~45 s (tick 2, 4, …) flip the office bulb
	if s.tick%2 == 0 {
		if e, ok := st["light.ampoule_4"]; ok {
			if e.State == "on" {
				e.State = "off"
				delete(e.Attributes, "brightness")
			} else {
				e.State = "on"
				e.Attributes["brightness"] = 204.0
			}
			e.LastChanged, e.LastUpdated = now, now
			changed = append(changed, e)
		}
	}
	if e, ok := st["sensor.smart_power_outlet_1_power"]; ok {
		e.State = fmt.Sprintf("%d", 80+s.rnd.Intn(12))
		e.LastUpdated = now
		changed = append(changed, e)
	}
	if e, ok := st[s.cfg.Globals.Sun]; ok {
		e.State = sunState(now)
		e.LastUpdated = now
		changed = append(changed, e)
	}
	s.Set(changed...)
}

func sunState(now time.Time) string {
	h := now.Hour()
	if h >= 8 && h < 17 {
		return "above_horizon"
	}
	return "below_horizon"
}

func nextRise(now time.Time) time.Time {
	t := time.Date(now.Year(), now.Month(), now.Day(), 7, 38, 0, 0, now.Location())
	if !t.After(now) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

func nextSet(now time.Time) time.Time {
	t := time.Date(now.Year(), now.Month(), now.Day(), 17, 21, 0, 0, now.Location())
	if !t.After(now) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

// CallService applies the service to the simulated entities so the interface
// behaves as with a real house (after a short, realistic delay).
func (s *Source) CallService(ctx context.Context, domain, service string, data map[string]any) error {
	ids := entityIDs(data["entity_id"])
	if len(ids) == 0 {
		return fmt.Errorf("mode démo : entity_id manquant")
	}
	now := time.Now()
	st := s.Snapshot()
	var changed []ha.Entity
	for _, id := range ids {
		e, ok := st[id]
		if !ok {
			// scripts / buttons that drive several entities
			if domain == "script" || domain == "button" || domain == "input_button" {
				changed = append(changed, s.applyScript(id, st, now)...)
				continue
			}
			return fmt.Errorf("mode démo : %s inconnu", id)
		}
		if e.State == "unavailable" {
			return fmt.Errorf("%s est injoignable", id)
		}
		if e.Attributes == nil {
			e.Attributes = map[string]any{}
		}
		switch domain + "." + service {
		case "light.turn_on":
			e.State = "on"
			if _, ok := e.Attributes["brightness"]; !ok {
				e.Attributes["brightness"] = 204.0
			}
			if v, ok := num(data["brightness_pct"]); ok {
				e.Attributes["brightness"] = math.Round(v / 100 * 255)
			}
			if v, ok := num(data["color_temp_kelvin"]); ok {
				e.Attributes["color_temp_kelvin"] = v
				delete(e.Attributes, "hs_color")
			}
			if hs, ok := data["hs_color"].([]float64); ok && len(hs) == 2 {
				e.Attributes["hs_color"] = []any{hs[0], hs[1]}
			}
		case "light.turn_off", "switch.turn_off", "input_boolean.turn_off", "homeassistant.turn_off":
			e.State = "off"
			if domain == "light" {
				delete(e.Attributes, "brightness")
			}
		case "switch.turn_on", "input_boolean.turn_on", "homeassistant.turn_on":
			e.State = "on"
		case "cover.open_cover":
			e.State = "open"
			e.Attributes["current_position"] = 100.0
		case "cover.close_cover":
			e.State = "closed"
			e.Attributes["current_position"] = 0.0
		case "cover.stop_cover":
		case "cover.set_cover_position":
			if v, ok := num(data["position"]); ok {
				e.Attributes["current_position"] = v
				if v == 0 {
					e.State = "closed"
				} else {
					e.State = "open"
				}
			}
		case "climate.set_temperature":
			if v, ok := num(data["temperature"]); ok {
				e.Attributes["temperature"] = v
			}
			if m, ok := data["hvac_mode"].(string); ok {
				e.State = m
			} else if e.State == "off" {
				e.State = "heat"
			}
			s.setZoneMode(id, st, "manual", &changed, now)
			s.recomputeHeating(&e)
		case "tado_hijack.set_mode":
			// e is the zone's mode sensor; the thermostat follows
			m, _ := data["hvac_mode"].(string)
			if c, ok := s.climateOf(id, st); ok {
				if v, ok := num(data["temperature"]); ok {
					c.Attributes["temperature"] = v
				}
				c.State = map[string]string{"heat": "heat", "off": "off", "auto": "auto"}[m]
				if c.State == "" {
					c.State = "heat"
				}
				s.recomputeHeating(&c)
				c.LastChanged, c.LastUpdated = now, now
				changed = append(changed, c)
			}
			e.State = map[string]string{"off": "off", "auto": "smart_schedule"}[m]
			if e.State == "" {
				e.State = "manual"
			}
		case "climate.set_hvac_mode":
			if m, ok := data["hvac_mode"].(string); ok {
				e.State = m
				if _, has := e.Attributes["temperature"]; !has && m != "off" {
					e.Attributes["temperature"] = 19.0
				}
				switch m {
				case "auto":
					s.setZoneMode(id, st, "smart_schedule", &changed, now)
				case "off":
					s.setZoneMode(id, st, "off", &changed, now)
				default:
					s.setZoneMode(id, st, "manual", &changed, now)
				}
			}
			s.recomputeHeating(&e)
		case "media_player.turn_off":
			e.State = "off"
			delete(e.Attributes, "app_name")
		case "media_player.media_stop":
			e.State = "idle"
		case "media_player.turn_on":
			if e.State == "off" {
				e.State = "idle"
			}
		case "media_player.media_next_track", "media_player.media_previous_track":
			by := 1
			if service == "media_previous_track" {
				by = -1
			}
			if !s.skipTrack(id, st, by) {
				return fmt.Errorf("mode démo : rien à passer sur %s", id)
			}
			continue
		case "media_player.media_seek":
			if v, ok := num(data["seek_position"]); ok {
				e.Attributes["media_position"] = v
				e.Attributes["media_position_updated_at"] = now.UTC().Format(time.RFC3339Nano)
			}
		case "media_player.media_play":
			e.State = "playing"
			if _, ok := e.Attributes["media_position"]; ok {
				e.Attributes["media_position_updated_at"] = now.UTC().Format(time.RFC3339Nano)
			}
		case "media_player.media_pause":
			if p, ok := num(e.Attributes["media_position"]); ok && e.State == "playing" {
				if at, err := time.Parse(time.RFC3339Nano, fmt.Sprint(e.Attributes["media_position_updated_at"])); err == nil {
					e.Attributes["media_position"] = math.Round((p+now.Sub(at).Seconds())*10) / 10
				}
				e.Attributes["media_position_updated_at"] = now.UTC().Format(time.RFC3339Nano)
			}
			e.State = "paused"
		case "media_player.volume_set":
			if v, ok := num(data["volume_level"]); ok {
				e.Attributes["volume_level"] = v
			}
		case "media_player.play_media":
			extra, _ := data["extra"].(map[string]any)
			meta, _ := extra["metadata"].(map[string]any)
			str := func(m map[string]any, k string) string { s, _ := m[k].(string); return s }
			e.State = "playing"
			e.Attributes["media_content_id"] = data["media_content_id"]
			e.Attributes["media_title"] = str(extra, "title")
			e.Attributes["media_artist"] = str(meta, "artist")
			e.Attributes["app_name"] = str(extra, "app_name")
			delete(e.Attributes, "media_channel")
		case "script.turn_on", "button.press", "input_button.press":
			changed = append(changed, s.applyScript(id, st, now)...)
			continue
		default:
			return fmt.Errorf("mode démo : %s.%s non simulé", domain, service)
		}
		e.LastChanged, e.LastUpdated = now, now
		changed = append(changed, e)
	}
	// a real device answers after a moment
	s.later(ctx, changed)
	return nil
}

func entityIDs(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []string:
		return x
	case []any:
		var out []string
		for _, i := range x {
			if s, ok := i.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func num(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	}
	return 0, false
}

// recomputeHeating: heating when the target is above the room temperature.
func (s *Source) recomputeHeating(e *ha.Entity) {
	t, _ := e.Attributes["temperature"].(float64)
	cur, _ := e.Attributes["current_temperature"].(float64)
	switch {
	case e.State == "off":
		e.Attributes["hvac_action"] = "off"
	case t > cur:
		e.Attributes["hvac_action"] = "heating"
	default:
		e.Attributes["hvac_action"] = "idle"
	}
}

// climateOf: the thermostat of the device whose zone mode sensor is zone.
func (s *Source) climateOf(zone string, st map[string]ha.Entity) (ha.Entity, bool) {
	for _, r := range s.cfg.Rooms {
		for _, d := range r.Devices {
			if d.Entities["zone_mode"] == zone {
				if c, ok := st[d.Entities["climate"]]; ok && c.Attributes != nil {
					return c, true
				}
			}
		}
	}
	return ha.Entity{}, false
}

// setZoneMode updates the Tado "zone mode" sensor of the device owning this climate.
func (s *Source) setZoneMode(climate string, st map[string]ha.Entity, mode string, changed *[]ha.Entity, now time.Time) {
	for _, r := range s.cfg.Rooms {
		for _, d := range r.Devices {
			if d.Entities["climate"] == climate && d.Entities["zone_mode"] != "" {
				if z, ok := st[d.Entities["zone_mode"]]; ok {
					z.State = mode
					z.LastChanged, z.LastUpdated = now, now
					*changed = append(*changed, z)
				}
			}
		}
	}
}

// applyScript simulates the house scripts: heating modes, camera privacy, alarm ack.
func (s *Source) applyScript(id string, st map[string]ha.Entity, now time.Time) []ha.Entity {
	g := s.cfg.Globals
	var out []ha.Entity
	setAll := func(hvac, zone string, target float64) {
		for _, r := range s.cfg.Rooms {
			for _, d := range r.Devices {
				if d.Kind != "heating" {
					continue
				}
				e, ok := st[d.Entities["climate"]]
				if !ok || e.State == "unavailable" {
					continue
				}
				e.State = hvac
				if target > 0 {
					e.Attributes["temperature"] = target
				}
				s.recomputeHeating(&e)
				e.LastChanged, e.LastUpdated = now, now
				out = append(out, e)
				if z, ok := st[d.Entities["zone_mode"]]; ok {
					z.State = zone
					z.LastChanged = now
					out = append(out, z)
				}
			}
		}
	}
	switch id {
	case g.Heating.Schedule:
		setAll("auto", "smart_schedule", 0)
	case g.Heating.Night:
		setAll("heat", "manual", 17)
	case g.Heating.Off:
		setAll("off", "off", 0)
	case g.Heating.Boost:
		setAll("heat", "manual", 25)
	case g.Cameras.Toggle:
		if e, ok := st[g.Cameras.Private]; ok {
			if e.State == "on" {
				e.State = "off"
			} else {
				e.State = "on"
			}
			e.LastChanged, e.LastUpdated = now, now
			out = append(out, e)
		}
	case g.Alarm.Ack:
		if e, ok := st[g.Alarm.Acked]; ok {
			e.State = "on"
			e.LastChanged, e.LastUpdated = now, now
			out = append(out, e)
		}
	}
	return out
}

// CameraSnapshot returns a tiny generated SVG "image" so the camera tiles
// have something to show.
func (s *Source) CameraSnapshot(ctx context.Context, entity string) ([]byte, string, error) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 360"><defs><radialGradient id="l" cx="70%" cy="40%" r="35%"><stop offset="0" stop-color="#ffd2a0" stop-opacity=".95"/><stop offset=".4" stop-color="#ffb478" stop-opacity=".5"/><stop offset="1" stop-color="#ffb478" stop-opacity="0"/></radialGradient><linearGradient id="w" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#2d2c35"/><stop offset="1" stop-color="#1a191f"/></linearGradient><linearGradient id="f" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#5e4c42"/><stop offset="1" stop-color="#2c2420"/></linearGradient></defs><rect width="640" height="360" fill="url(#w)"/><g stroke="#ffffff" stroke-opacity=".04">` +
		`<path d="M52 0v240M104 0v240M156 0v240M208 0v240M260 0v240M312 0v240M364 0v240M416 0v240M468 0v240M520 0v240M572 0v240"/></g><rect y="240" width="640" height="120" fill="url(#f)"/><rect width="640" height="360" fill="url(#l)"/><text x="16" y="344" font-family="system-ui" font-size="14" fill="#fff" fill-opacity=".6">démo · ` + entity + `</text></svg>`
	return []byte(svg), "image/svg+xml", nil
}

func (s *Source) CameraStream(ctx context.Context, entity string, w io.Writer, setHeader func(string)) error {
	return fmt.Errorf("mode démo : pas de flux vidéo")
}

// Air is the demo air-quality fetcher: a plausible autumn day in the Lyon area.
type Air struct{}

func (Air) Fetch(ctx context.Context, lat, lon float64) (model.Air, error) {
	now := time.Now()
	cur := map[string]any{
		"european_aqi": 34.0,
		"pm2_5":        9.0, "european_aqi_pm2_5": 18.0,
		"pm10": 16.0, "european_aqi_pm10": 16.0,
		"nitrogen_dioxide": 31.0, "european_aqi_nitrogen_dioxide": 24.0,
		"ozone": 68.0, "european_aqi_ozone": 34.0,
		"sulphur_dioxide": 2.0, "european_aqi_sulphur_dioxide": 2.0,
		"grass_pollen": 0.0, "birch_pollen": 0.0, "alder_pollen": 4.0, "mugwort_pollen": 0.0, "olive_pollen": 0.0, "ragweed_pollen": 0.0,
	}
	tomorrow := now.AddDate(0, 0, 1).Format("2006-01-02")
	times := []string{tomorrow + "T08:00", tomorrow + "T14:00", tomorrow + "T20:00"}
	a := air.FromOpenMeteo(cur, times, []float64{28, 45, 38}, now)
	a.Source = "démo"
	return a, nil
}

// SeedAgenda fills an in-memory agenda with a plausible family life around
// `now`: weekly routines, a fortnightly lunch, the bins, a birthday.
func SeedAgenda(st *agenda.Store, calendars []string, now time.Time) {
	fam, bins := "", ""
	for i, c := range calendars {
		if i == 0 {
			fam = c
		}
		if c == "poubelles" {
			bins = c
		}
	}
	if bins == "" {
		bins = fam
	}
	monday := now
	for monday.Weekday() != time.Monday {
		monday = monday.AddDate(0, 0, -1)
	}
	d := func(off int) string { return monday.AddDate(0, 0, off).Format("2006-01-02") }
	weekly := &agenda.Repeat{Freq: agenda.Weekly}
	for _, e := range []agenda.Event{
		{Calendar: fam, Title: "Crèche · dépôt", Location: "Crèche des Lilas", Date: d(0), Start: "08:15", End: "08:45", Repeat: weekly},
		{Calendar: fam, Title: "Bébé nageur", Location: "Piscine de Vénissieux", Date: d(2), Start: "17:30", End: "18:15", Repeat: weekly},
		{Calendar: fam, Title: "Courses", Date: d(5), Start: "10:00", End: "11:30", Repeat: weekly},
		{Calendar: fam, Title: "Déjeuner chez les parents", Location: "Lyon 8e", Date: d(6), Start: "12:00", End: "15:00", Repeat: &agenda.Repeat{Freq: agenda.Biweekly}},
		{Calendar: fam, Title: "Pédiatre · vaccin", Location: "Cabinet Dr Martin", Date: d(3), Start: "09:30", End: "10:00"},
		{Calendar: fam, Title: "Anniversaire de Nora", Date: d(4), AllDay: true, Repeat: &agenda.Repeat{Freq: agenda.Yearly}},
		{Calendar: fam, Title: "Plombier · chaudière", Location: "Maison", Date: d(8), Start: "14:00", End: "16:00"},
		{Calendar: bins, Title: "Poubelle jaune", Date: d(1), AllDay: true, Repeat: weekly},
		{Calendar: bins, Title: "Verre", Date: d(4), AllDay: true, Repeat: &agenda.Repeat{Freq: agenda.Biweekly}},
	} {
		if err := e.Validate(calendars); err == nil {
			_, _ = st.Add(e, "Démo")
		}
	}
}

// Music is the demo music search: a small catalogue, searched on title,
// artist and album, played like Music Assistant would (the speaker's
// entities.music player gets the queue). Nothing leaves the machine.
type Music struct{ Src *Source }

var catalogue = []model.Track{
	{URI: "spotify://track/demo1", Title: "One More Time", Artists: "Daft Punk", Album: "Discovery", Duration: 320000},
	{URI: "spotify://track/demo3", Title: "Harder, Better, Faster, Stronger", Artists: "Daft Punk", Album: "Discovery", Duration: 224000},
	{URI: "spotify://track/demo2", Title: "Get Lucky", Artists: "Daft Punk, Pharrell Williams", Album: "Random Access Memories", Duration: 369000},
	{URI: "spotify://track/demo5", Title: "Instant Crush", Artists: "Daft Punk, Julian Casablancas", Album: "Random Access Memories", Duration: 337000},
	{URI: "spotify://track/demo4", Title: "Around the World", Artists: "Daft Punk", Album: "Homework", Duration: 429000},
	{URI: "spotify://track/demo6", Title: "Alors on danse", Artists: "Stromae", Album: "Cheese", Duration: 206000},
	{URI: "spotify://track/demo7", Title: "La Javanaise", Artists: "Serge Gainsbourg", Album: "Gainsbourg Confidentiel", Duration: 147000},
	{URI: "spotify://track/demo8", Title: "Nightcall", Artists: "Kavinsky", Album: "OutRun", Duration: 258000},
}

var albums = []model.Album{
	{URI: "spotify://album/discovery", Title: "Discovery", Artists: "Daft Punk", Year: 2001},
	{URI: "spotify://album/ram", Title: "Random Access Memories", Artists: "Daft Punk", Year: 2013},
	{URI: "spotify://album/homework", Title: "Homework", Artists: "Daft Punk", Year: 1997},
	{URI: "spotify://album/cheese", Title: "Cheese", Artists: "Stromae", Year: 2010},
	{URI: "spotify://album/outrun", Title: "OutRun", Artists: "Kavinsky", Year: 2013},
}

func (Music) Search(ctx context.Context, q string, limit int) (model.Results, error) {
	q = strings.ToLower(strings.TrimSpace(q))
	out := model.Results{Tracks: []model.Track{}, Albums: []model.Album{}}
	if q == "" {
		return out, nil
	}
	for _, t := range catalogue {
		if strings.Contains(strings.ToLower(t.Title+" "+t.Artists+" "+t.Album), q) && len(out.Tracks) < limit {
			out.Tracks = append(out.Tracks, t)
		}
	}
	for _, a := range albums {
		if strings.Contains(strings.ToLower(a.Title+" "+a.Artists), q) && len(out.Albums) < (limit+1)/2 {
			out.Albums = append(out.Albums, a)
		}
	}
	return out, nil
}

func (Music) Source() string { return "Music Assistant (démo)" }

func (m Music) Play(ctx context.Context, room config.Room, dev config.Device, uri, kind string) error {
	i := -1
	for j, t := range catalogue {
		if t.URI == uri {
			i = j
		}
	}
	if kind == "album" {
		for _, a := range albums {
			if a.URI == uri {
				for j, t := range catalogue {
					if t.Album == a.Title {
						i = j
						break
					}
				}
			}
		}
	}
	if i < 0 {
		return fmt.Errorf("mode démo : titre inconnu %s", uri)
	}
	if m.Src == nil || dev.Entities["music"] == "" {
		return nil
	}
	return m.Src.playTrack(dev, i)
}

// massFeatures: pause, seek, volume, previous, next, play_media, play, stop…
const massFeatures = 1 | 2 | 4 | 16 | 32 | 512 | 4096 | 16384

// playTrack puts catalogue[i] on the speaker's Music Assistant player, like
// MA streaming to a Cast speaker (whose app becomes "Music Assistant").
func (s *Source) playTrack(dev config.Device, i int) error {
	now := time.Now()
	st := s.Snapshot()
	mp, ok := st[dev.Entities["music"]]
	if !ok {
		return fmt.Errorf("mode démo : %s inconnu", dev.Entities["music"])
	}
	t := catalogue[(i+len(catalogue))%len(catalogue)]
	mp.State = "playing"
	mp.Attributes = map[string]any{"mass_player_type": "player", "supported_features": float64(massFeatures), "volume_level": 0.35,
		"media_title": t.Title, "media_artist": t.Artists, "media_album_name": t.Album, "media_content_id": t.URI, "media_content_type": "music",
		"media_duration": float64(t.Duration / 1000), "media_position": 0.0, "media_position_updated_at": now.UTC().Format(time.RFC3339Nano)}
	mp.LastChanged, mp.LastUpdated = now, now
	changed := []ha.Entity{mp}
	if c, ok := st[dev.Entities["media_player"]]; ok {
		c.State = "playing"
		vol := c.Attributes["volume_level"]
		c.Attributes = map[string]any{"app_name": "Music Assistant", "media_title": t.Title, "volume_level": vol}
		c.LastChanged, c.LastUpdated = now, now
		changed = append(changed, c)
	}
	s.later(context.Background(), changed)
	return nil
}

// skipTrack: the previous / next catalogue track on a Music Assistant player.
func (s *Source) skipTrack(id string, st map[string]ha.Entity, by int) bool {
	cur, _ := st[id].Attributes["media_content_id"].(string)
	for _, r := range s.cfg.Rooms {
		for _, d := range r.Devices {
			if d.Entities["music"] != id {
				continue
			}
			for j, t := range catalogue {
				if t.URI == cur {
					_ = s.playTrack(d, j+by)
					return true
				}
			}
		}
	}
	return false
}
