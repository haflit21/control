// Package build turns Home Assistant entity states into the model the
// interface reads: pure functions of (config, states, forecasts, now).
// Every HA value is normalised here (on/off, °C, %, 0-100) and never
// passed through as a raw string.
package build

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"math"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"ahomelab.io/control/internal/config"
	"ahomelab.io/control/internal/ha"
	"ahomelab.io/control/internal/model"
)

// Input is everything Build needs.
type Input struct {
	Config    *config.Config
	States    map[string]ha.Entity
	Forecasts map[string][]map[string]any // for the weather entity
	Now       time.Time
	Connected bool
	Demo      bool
	Air       *model.Air    // outdoor air quality, polled separately (nil = not yet)
	Agenda    *model.Agenda // shared calendar, polled separately (nil = none configured)
	Music     string        // music search source ("Music Assistant", "Spotify"), "" = none
	// HeatPreset: the house heating mode last chosen in Control, and when
	HeatPreset   string
	HeatPresetAt time.Time
}

// Build computes the whole house.
func Build(in Input) model.Home {
	cfg := in.Config
	st := in.States
	h := model.Home{Name: cfg.Name, Connected: in.Connected, Demo: in.Demo, UpdatedAt: in.Now, Alerts: []model.Alert{}}
	if len(cfg.Media.Radios) > 0 || in.Music != "" {
		h.Media = &model.Media{Radios: []model.Radio{}, Music: in.Music != "", MusicSource: in.Music}
		for _, r := range cfg.Media.Radios {
			rd := model.Radio{ID: r.ID, Name: r.Name, Genre: r.Genre, Short: r.Short, Color: r.Color}
			if r.Logo != "" {
				rd.Logo = "/" + r.Logo
			}
			h.Media.Radios = append(h.Media.Radios, rd)
		}
	}
	var allAlerts []model.Alert
	unavailable := []unav{}
	for _, rc := range cfg.Rooms {
		room := model.Room{ID: rc.ID, Name: rc.Name, Icon: rc.Icon, Image: mediaURL(rc.Image), Devices: []model.Device{}}
		room.Summary.Alerts = []model.Alert{}
		var primary *model.Device
		var heatingAny bool
		for _, dc := range rc.Devices {
			d := device(rc.ID, dc, st, in.Now)
			if d.Kind == model.KindHeating {
				expectHeating(&d, cfg, rc.ID, dc, st, in)
			}
			if d.Kind == model.KindMedia {
				mediaExtras(&d, cfg, dc, st)
			}
			if !d.Available {
				since := in.Now
				if e, ok := st[mainEntity(dc)]; ok && !e.LastChanged.IsZero() {
					since = e.LastChanged
				}
				unavailable = append(unavailable, unav{room: rc.Name, name: d.Name, since: since, roomID: rc.ID, devID: d.ID})
			}
			if dc.Hidden {
				// hidden devices still feed the room summary (a hidden radiator head heats the room)
				if d.Kind == model.KindHeating && d.State.Heating != nil && *d.State.Heating {
					heatingAny = true
				}
				continue
			}
			room.Devices = append(room.Devices, d)
			last := &room.Devices[len(room.Devices)-1]
			if dc.Primary {
				primary = last
			}
			switch d.Kind {
			case model.KindHeating:
				if d.State.Heating != nil && *d.State.Heating {
					heatingAny = true
				}
			case model.KindLight, model.KindLightGroup:
				if d.Kind == model.KindLight && d.State.On != nil && *d.State.On {
					room.Summary.LightsOn++
				}
			case model.KindOpening:
				if d.State.Open != nil && *d.State.Open {
					room.Summary.OpeningsOpen++
				}
			}
			if !d.Available {
				room.Summary.Unavailable++
			}
			if d.Battery == "low" {
				allAlerts = append(allAlerts, model.Alert{ID: d.ID + ".battery", Level: "warn", Kind: "battery", Title: "Pile faible", Detail: d.Name + " · " + rc.Name, RoomID: rc.ID, DeviceID: d.ID, Since: d.UpdatedAt})
			}
			if d.Kind == model.KindSmoke {
				if d.State.Smoke != nil && *d.State.Smoke {
					allAlerts = append(allAlerts, model.Alert{ID: d.ID + ".smoke", Level: "crit", Kind: "smoke", Title: "Fumée détectée dans " + lower(rc.Name), Detail: d.Name, RoomID: rc.ID, DeviceID: d.ID, Since: since(st, dc.Entities["smoke"], in.Now)})
				}
				if d.State.CO != nil && *d.State.CO {
					allAlerts = append(allAlerts, model.Alert{ID: d.ID + ".co", Level: "crit", Kind: "co", Title: "Monoxyde de carbone détecté dans " + lower(rc.Name), Detail: d.Name, RoomID: rc.ID, DeviceID: d.ID, Since: since(st, dc.Entities["co"], in.Now)})
				}
			}
		}
		if primary == nil {
			for i := range room.Devices {
				if room.Devices[i].Kind == model.KindHeating || room.Devices[i].Kind == model.KindClimateSensor {
					primary = &room.Devices[i]
					break
				}
			}
		}
		if primary != nil {
			primary.Primary = true
			room.Summary.Temperature = primary.State.Temperature
			room.Summary.Humidity = primary.State.Humidity
			room.Summary.Target = primary.State.Target
			room.Summary.HeatingMode = primary.State.Mode
		}
		room.Summary.Heating = heatingAny
		if heatingAny && room.Summary.OpeningsOpen > 0 {
			for _, d := range room.Devices {
				if d.Kind == model.KindOpening && d.State.Open != nil && *d.State.Open {
					detail := ""
					if room.Summary.Target != nil {
						detail = fmt.Sprintf("consigne %s", degree(*room.Summary.Target))
					}
					a := model.Alert{ID: d.ID + ".window_heating", Level: "warn", Kind: "window_heating", Title: d.Name + " ouverte pendant que " + article(rc.Name) + " chauffe", Detail: detail, RoomID: rc.ID, DeviceID: d.ID, Since: parseTime(d.State.Since, in.Now)}
					room.Summary.Alerts = append(room.Summary.Alerts, a)
					allAlerts = append(allAlerts, a)
				}
			}
		}
		h.Rooms = append(h.Rooms, room)
	}
	// unavailable devices: one alert, after an hour
	var old []unav
	for _, u := range unavailable {
		if in.Now.Sub(u.since) > time.Hour {
			old = append(old, u)
		}
	}
	if len(old) > 0 {
		sort.Slice(old, func(i, j int) bool { return old[i].since.Before(old[j].since) })
		names := make([]string, 0, len(old))
		for _, u := range old {
			names = append(names, lower(u.name)+" "+lower(u.room))
		}
		title := "1 appareil injoignable"
		if len(old) > 1 {
			title = fmt.Sprintf("%d appareils injoignables", len(old))
		}
		a := model.Alert{ID: "home.unavailable", Level: "info", Kind: "unavailable", Title: title, Detail: upperFirst(strings.Join(names, ", ")), Since: old[0].since}
		if len(old) == 1 {
			a.RoomID, a.DeviceID = old[0].roomID, old[0].devID
		}
		allAlerts = append(allAlerts, a)
	}
	if !in.Connected && !in.Demo {
		allAlerts = append(allAlerts, model.Alert{ID: "home.offline", Level: "info", Kind: "offline", Title: "Connexion à Home Assistant perdue", Detail: "Les valeurs affichées ne sont plus à jour", Since: in.Now})
	}
	h.Globals = globals(cfg, st, h.Rooms, in)
	h.Presence = presence(cfg, st)
	h.Weather = weather(cfg, st, in.Forecasts, in.Now)
	h.Air = in.Air
	h.Agenda = in.Agenda
	if a := in.Air; a != nil && a.Available && a.Index >= 4 {
		detail := "Indice " + strconv.Itoa(a.Index) + ", " + lower(a.Label)
		if name := pollutantName(a.Dominant); name != "" {
			detail += " · " + name
		}
		allAlerts = append(allAlerts, model.Alert{ID: "home.air", Level: "warn", Kind: "air", Title: "Épisode de pollution de l'air", Detail: detail + " · aérer tôt le matin", Since: a.UpdatedAt})
	}
	h.Alerts = sortAlerts(allAlerts)
	return h
}

func pollutantName(key string) string {
	switch key {
	case "pm25":
		return "particules fines PM₂.₅"
	case "pm10":
		return "particules PM₁₀"
	case "no2":
		return "dioxyde d'azote"
	case "o3":
		return "ozone"
	case "so2":
		return "dioxyde de soufre"
	}
	return ""
}

type unav struct {
	room, name, roomID, devID string
	since                     time.Time
}

func mainEntity(dc config.Device) string {
	for _, role := range []string{"climate", "light", "cover", "switch", "contact", "camera", "media_player", "smoke", "motion", "temperature", "humidity"} {
		if e := dc.Entities[role]; e != "" {
			return e
		}
	}
	return ""
}

// device normalises one configured device from its entities.
func device(roomID string, dc config.Device, st map[string]ha.Entity, now time.Time) model.Device {
	d := model.Device{ID: roomID + "." + dc.ID, RoomID: roomID, Kind: dc.Kind, Name: dc.Name, Image: mediaURL(dc.Image), ImageOff: mediaURL(dc.ImageOff), Capabilities: []string{}, Available: true}
	get := func(role string) (ha.Entity, bool) {
		e, ok := st[dc.Entities[role]]
		return e, ok && dc.Entities[role] != ""
	}
	main, hasMain := get(roleOf(dc.Kind))
	if hasMain {
		d.Available = available(main)
		d.UpdatedAt = main.LastUpdated
	} else if dc.Entities[roleOf(dc.Kind)] != "" {
		d.Available = false // configured but HA never sent it
	}
	for _, e := range dc.Entities {
		if x, ok := st[e]; ok && x.LastUpdated.After(d.UpdatedAt) {
			d.UpdatedAt = x.LastUpdated
		}
	}
	if d.UpdatedAt.IsZero() {
		d.UpdatedAt = now
	}
	if b, ok := get("battery"); ok {
		d.Battery = "ok"
		if b.State == "on" {
			d.Battery = "low"
		}
	}
	s := &d.State
	switch dc.Kind {
	case model.KindHeating:
		d.Capabilities = []string{"set_target", "set_mode"}
		if t, ok := get("temperature"); ok {
			s.Temperature = round1(stateNum(t))
		} else {
			s.Temperature = round1(attrNum(main, "current_temperature"))
		}
		if hm, ok := get("humidity"); ok {
			s.Humidity = round0(stateNum(hm))
		} else {
			s.Humidity = round0(attrNum(main, "current_humidity"))
		}
		s.Target = round1(attrNum(main, "temperature"))
		action, _ := main.Attributes["hvac_action"].(string)
		s.Heating = model.B(action == "heating" || action == "preheating")
		if main.State == "off" || main.State == "unavailable" {
			s.Target = nil
		}
		s.Mode = heatingMode(main, dc, st)
		if s.Mode == "off" {
			s.Target = nil
			s.Heating = model.B(false)
		}
		if zm, ok := get("zone_mode"); ok {
			for _, k := range []string{"overlay_end", "end", "until", "expires"} {
				if v, ok := zm.Attributes[k].(string); ok && v != "" {
					s.ModeUntil = v
					break
				}
			}
		}
		if p, ok := get("heating_power"); ok {
			s.HeatingPower = round0(stateNum(p))
		}
		if m, ok := get("mold_risk"); ok && available(m) {
			s.MoldRisk = model.B(m.State == "on")
		}
	case model.KindAC:
		d.Capabilities = []string{"power", "set_mode", "set_target", "set_eco", "set_boost"}
		s.On = model.B(main.State != "off" && available(main))
		s.Mode = acMode(main.State)
		s.Temperature = round1(attrNum(main, "current_temperature"))
		s.Target = round1(attrNum(main, "temperature"))
		if e, ok := get("eco"); ok {
			s.Eco = model.B(e.State == "on")
		}
		if b, ok := get("boost"); ok {
			s.Boost = model.B(b.State == "on")
		}
		if p, ok := get("power"); ok {
			s.Power = round0(stateNum(p))
		}
	case model.KindLight, model.KindLightGroup:
		d.Capabilities = []string{"power", "set_brightness"}
		s.On = model.B(main.State == "on")
		if b := attrNum(main, "brightness"); b != nil {
			s.Brightness = model.I(int(math.Round(*b / 255 * 100)))
		}
		if modes, ok := main.Attributes["supported_color_modes"].([]any); ok {
			for _, m := range modes {
				switch m {
				case "color_temp":
					d.Capabilities = append(d.Capabilities, "set_color_temp")
				case "hs", "xy", "rgb":
					d.Capabilities = append(d.Capabilities, "set_color")
				}
			}
		}
		if k := attrNum(main, "color_temp_kelvin"); k != nil {
			s.ColorTemp = model.I(int(*k))
		}
		if hs, ok := main.Attributes["hs_color"].([]any); ok && len(hs) == 2 {
			h, _ := toFloat(hs[0])
			sat, _ := toFloat(hs[1])
			s.Color = &model.HS{H: h, S: sat}
		}
		s.Since = main.LastChanged.Format(time.RFC3339)
	case model.KindCover:
		d.Capabilities = []string{"open", "close", "stop", "set_position"}
		if p := attrNum(main, "current_position"); p != nil {
			s.Position = model.I(int(math.Round(*p)))
		} else if main.State == "open" {
			s.Position = model.I(100)
		} else if main.State == "closed" {
			s.Position = model.I(0)
		}
		if main.State == "opening" || main.State == "closing" {
			s.Moving = main.State
		}
	case model.KindPlug:
		d.Capabilities = []string{"power"}
		s.On = model.B(main.State == "on")
		if p, ok := get("power"); ok {
			s.Power = round0(stateNum(p))
		}
		s.Since = main.LastChanged.Format(time.RFC3339)
	case model.KindOpening:
		s.Open = model.B(main.State == "on")
		s.Since = main.LastChanged.Format(time.RFC3339)
	case model.KindCamera:
		d.Capabilities = []string{"set_privacy"}
		if m, ok := get("motion"); ok {
			s.Motion = model.B(m.State == "on")
		}
		if p, ok := get("privacy"); ok {
			s.Privacy = model.B(p.State == "on")
		}
	case model.KindMedia:
		d.Capabilities = []string{"play", "pause", "set_volume", "power"}
		switch main.State {
		case "playing", "paused", "idle":
			s.Playing = main.State
		case "buffering":
			s.Playing = "playing"
		case "on", "standby":
			s.Playing = "idle"
		default:
			s.Playing = "off"
		}
		s.On = model.B(s.Playing != "off")
		if v := attrNum(main, "volume_level"); v != nil {
			s.Volume = model.I(int(math.Round(*v * 100)))
		}
		s.Title, _ = main.Attributes["media_title"].(string)
		s.Artist, _ = main.Attributes["media_artist"].(string)
		if s.Artist == "" {
			s.Artist, _ = main.Attributes["media_channel"].(string)
		}
	case model.KindSmoke:
		s.Smoke = model.B(main.State == "on")
		if c, ok := get("co"); ok {
			s.CO = model.B(c.State == "on")
		}
	case model.KindMotion:
		s.Motion = model.B(main.State == "on")
		s.Since = main.LastChanged.Format(time.RFC3339)
	case model.KindClimateSensor:
		if t, ok := get("temperature"); ok {
			s.Temperature = round1(stateNum(t))
			d.Available = available(t)
			d.UpdatedAt = t.LastUpdated
		}
		if hm, ok := get("humidity"); ok {
			s.Humidity = round0(stateNum(hm))
		}
	}
	return d
}

func roleOf(k model.DeviceKind) string {
	switch k {
	case model.KindHeating, model.KindAC:
		return "climate"
	case model.KindLight, model.KindLightGroup:
		return "light"
	case model.KindCover:
		return "cover"
	case model.KindPlug:
		return "switch"
	case model.KindOpening:
		return "contact"
	case model.KindCamera:
		return "camera"
	case model.KindMedia:
		return "media_player"
	case model.KindSmoke:
		return "smoke"
	case model.KindMotion:
		return "motion"
	}
	return "temperature"
}

// expectSpan: how long a house mode chosen in Control is shown before Tado
// confirms it (the zone modes come from the tado cloud, a few minutes late).
const expectSpan = 4 * time.Minute

// expectHeating shows the house mode just chosen in Control on a zone until
// tado confirms it: the zone mode sensor has not changed since the command.
func expectHeating(d *model.Device, cfg *config.Config, roomID string, dc config.Device, st map[string]ha.Entity, in Input) {
	if in.HeatPreset == "" || in.Now.Sub(in.HeatPresetAt) > expectSpan || !d.Available {
		return
	}
	if zm, ok := st[dc.Entities["zone_mode"]]; ok && zm.LastChanged.After(in.HeatPresetAt) {
		return // tado answered: its mode is the truth
	}
	s := &d.State
	switch in.HeatPreset {
	case "off":
		s.Mode, s.Target, s.Heating = "off", nil, model.B(false)
	case "schedule":
		s.Mode = "schedule"
	case "boost":
		s.Mode, s.Target = "manual", model.F(25)
	case "night":
		h := cfg.Globals.Heating
		if len(h.NightRooms) == 0 {
			s.Mode = "manual"
			return
		}
		for _, r := range h.NightRooms {
			if r == roomID {
				s.Mode = "manual"
				if h.NightTarget > 0 {
					s.Target = model.F(h.NightTarget)
				}
				return
			}
		}
		s.Mode, s.Target, s.Heating = "off", nil, model.B(false)
	}
}

// heatingMode: Tado Hijack zone mode when present, else the climate state.
func heatingMode(main ha.Entity, dc config.Device, st map[string]ha.Entity) string {
	if zm, ok := st[dc.Entities["zone_mode"]]; ok && available(zm) {
		v := strings.ToLower(zm.State)
		switch {
		case strings.Contains(v, "off"):
			return "off"
		case strings.Contains(v, "auto"), strings.Contains(v, "sched"), strings.Contains(v, "smart"), strings.Contains(v, "planning"):
			return "schedule"
		case strings.Contains(v, "manual"), strings.Contains(v, "overlay"), strings.Contains(v, "timer"):
			return "manual"
		}
	}
	switch main.State {
	case "off", "unavailable", "unknown":
		return "off"
	case "auto":
		return "schedule"
	}
	return "manual"
}

func acMode(state string) string {
	switch state {
	case "cool", "heat", "dry", "auto":
		return state
	case "fan_only":
		return "fan"
	}
	return "off"
}

func globals(cfg *config.Config, st map[string]ha.Entity, rooms []model.Room, in Input) model.Globals {
	var g model.Globals
	g.Heating.Available = true
	if e, ok := st[cfg.Globals.Average.Temperature]; ok && available(e) {
		g.Heating.Average = round1(stateNum(e))
	}
	if e, ok := st[cfg.Globals.Average.Humidity]; ok && available(e) {
		g.Heating.Humidity = round0(stateNum(e))
	}
	modes := map[string]int{}
	for _, r := range rooms {
		if r.Summary.Heating {
			g.Heating.RoomsOn++
		}
		g.LightsOn += r.Summary.LightsOn
		g.OpeningsOpen += r.Summary.OpeningsOpen
		for _, d := range r.Devices {
			if d.Kind == model.KindHeating && d.Available {
				modes[d.State.Mode]++
			}
		}
	}
	switch {
	case len(modes) == 0:
		g.Heating.Available = false
	case len(modes) == 1:
		for m := range modes {
			g.Heating.Mode = m
		}
	default:
		g.Heating.Mode = "mixed"
	}
	g.Heating.Preset = preset(in, modes)
	if e, ok := st[cfg.Globals.Heating.Presence]; ok && available(e) {
		v := strings.ToLower(e.State)
		switch {
		case strings.Contains(v, "home"), strings.Contains(v, "maison"):
			g.Heating.Presence = "home"
		case strings.Contains(v, "away"), strings.Contains(v, "absent"):
			g.Heating.Presence = "away"
		default:
			g.Heating.Presence = "auto"
		}
	}
	if e, ok := st[cfg.Globals.Heating.Quota]; ok && available(e) {
		if n := stateNum(e); n != nil {
			g.Heating.APIQuota = model.I(int(*n))
		}
	}
	if e, ok := st[cfg.Globals.Alarm.Armed]; ok {
		g.Alarm.Available = available(e)
		g.Alarm.Armed = e.State == "on"
	}
	if e, ok := st[cfg.Globals.Alarm.Acked]; ok {
		g.Alarm.Acked = e.State == "on"
	}
	if e, ok := st[cfg.Globals.Alarm.Unifi]; ok && available(e) {
		g.Alarm.UnifiState = e.State
		g.Alarm.Triggered = e.State == "triggered"
	}
	if sw := cfg.Globals.Cameras.Privacy; len(sw) > 0 {
		// the cameras' own privacy switches: private as soon as one is on
		known, on := false, false
		for _, id := range sw {
			if e, ok := st[id]; ok && available(e) {
				known = true
				on = on || e.State == "on"
			}
		}
		if known {
			g.CamerasPrivate = model.B(on)
		}
	} else if e, ok := st[cfg.Globals.Cameras.Private]; ok && available(e) {
		g.CamerasPrivate = model.B(e.State == "on")
	}
	return g
}

func presence(cfg *config.Config, st map[string]ha.Entity) []model.Person {
	out := []model.Person{}
	for _, p := range cfg.Globals.Presence {
		e := st[p.Entity]
		name := p.Name
		if name == "" {
			name, _ = e.Attributes["friendly_name"].(string)
		}
		out = append(out, model.Person{ID: strings.TrimPrefix(p.Entity, "person."), Name: name, Home: e.State == "home"})
	}
	return out
}

func weather(cfg *config.Config, st map[string]ha.Entity, fc map[string][]map[string]any, now time.Time) *model.Weather {
	if cfg.Globals.Weather == "" {
		return nil
	}
	e, ok := st[cfg.Globals.Weather]
	w := &model.Weather{Hourly: []model.ForecastHour{}, Daily: []model.ForecastDay{}, UpdatedAt: e.LastUpdated}
	if !ok || !available(e) {
		w.Available = false
		return w
	}
	w.Available = true
	w.Condition = e.State
	w.Temperature = round0(attrNum(e, "temperature"))
	w.Feels = round0(attrNum(e, "apparent_temperature"))
	w.Humidity = round0(attrNum(e, "humidity"))
	w.Pressure = round0(attrNum(e, "pressure"))
	w.WindSpeed = round0(attrNum(e, "wind_speed"))
	w.WindBearing = round0(attrNum(e, "wind_bearing"))
	w.WindGust = round0(attrNum(e, "wind_gust_speed"))
	w.UV = round0(attrNum(e, "uv_index"))
	w.Visibility = round0(attrNum(e, "visibility"))
	w.CloudCover = round0(attrNum(e, "cloud_coverage"))
	w.DewPoint = round0(attrNum(e, "dew_point"))
	w.Place, _ = e.Attributes["friendly_name"].(string)
	if sun, ok := st[cfg.Globals.Sun]; ok && available(sun) {
		w.Night = sun.State == "below_horizon"
		w.Sunrise, _ = sun.Attributes["next_rising"].(string)
		w.Sunset, _ = sun.Attributes["next_setting"].(string)
	} else {
		w.Night = e.State == "clear-night" || now.Hour() < 7 || now.Hour() >= 19
	}
	for _, it := range fc["hourly"] {
		h := model.ForecastHour{Time: str(it["datetime"]), Condition: str(it["condition"])}
		h.Temperature = round0(toFloatP(it["temperature"]))
		h.Precipitation = round1(toFloatP(it["precipitation"]))
		h.PrecipProb = round0(toFloatP(it["precipitation_probability"]))
		h.WindSpeed = round0(toFloatP(it["wind_speed"]))
		w.Hourly = append(w.Hourly, h)
		if len(w.Hourly) >= 24 {
			break
		}
	}
	for _, it := range fc["daily"] {
		d := model.ForecastDay{Date: str(it["datetime"]), Condition: str(it["condition"])}
		d.Max = round0(toFloatP(it["temperature"]))
		d.Min = round0(toFloatP(it["templow"]))
		d.Precipitation = round1(toFloatP(it["precipitation"]))
		d.PrecipProb = round0(toFloatP(it["precipitation_probability"]))
		d.WindSpeed = round0(toFloatP(it["wind_speed"]))
		d.WindBearing = round0(toFloatP(it["wind_bearing"]))
		d.Humidity = round0(toFloatP(it["humidity"]))
		d.UV = round0(toFloatP(it["uv_index"]))
		w.Daily = append(w.Daily, d)
		if len(w.Daily) >= 7 {
			break
		}
	}
	if len(w.Daily) > 0 {
		w.Min, w.Max = w.Daily[0].Min, w.Daily[0].Max
	}
	return w
}

func sortAlerts(a []model.Alert) []model.Alert {
	rank := map[string]int{"crit": 0, "warn": 1, "info": 2}
	sort.SliceStable(a, func(i, j int) bool {
		if rank[a[i].Level] != rank[a[j].Level] {
			return rank[a[i].Level] < rank[a[j].Level]
		}
		return a[i].Since.Before(a[j].Since)
	})
	if a == nil {
		return []model.Alert{}
	}
	return a
}

// ---- helpers ----

func available(e ha.Entity) bool {
	return e.EntityID != "" && e.State != "unavailable" && e.State != "unknown" && e.State != ""
}

func stateNum(e ha.Entity) *float64 {
	if !available(e) {
		return nil
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(e.State), 64)
	if err != nil {
		return nil
	}
	return &f
}

func attrNum(e ha.Entity, key string) *float64 {
	v, ok := e.Attributes[key]
	if !ok || v == nil {
		return nil
	}
	return toFloatP(v)
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case string:
		f, err := strconv.ParseFloat(x, 64)
		return f, err == nil
	}
	return 0, false
}

func toFloatP(v any) *float64 {
	f, ok := toFloat(v)
	if !ok {
		return nil
	}
	return &f
}

func round1(p *float64) *float64 {
	if p == nil || math.IsNaN(*p) {
		return nil
	}
	v := math.Round(*p*10) / 10
	return &v
}

func round0(p *float64) *float64 {
	if p == nil || math.IsNaN(*p) {
		return nil
	}
	v := math.Round(*p)
	return &v
}

func str(v any) string { s, _ := v.(string); return s }

func since(st map[string]ha.Entity, entity string, now time.Time) time.Time {
	if e, ok := st[entity]; ok && !e.LastChanged.IsZero() {
		return e.LastChanged
	}
	return now
}

func parseTime(s string, fallback time.Time) time.Time {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return fallback
}

func degree(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	return strings.Replace(s, ".", ",", 1) + "°"
}

func lower(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// article: "la cuisine", "le salon", "l'entrée" — good enough for room names.
func article(name string) string {
	l := strings.ToLower(name)
	switch {
	case strings.HasPrefix(l, "a"), strings.HasPrefix(l, "e"), strings.HasPrefix(l, "é"), strings.HasPrefix(l, "i"), strings.HasPrefix(l, "o"), strings.HasPrefix(l, "u"):
		return "l'" + l
	case strings.HasSuffix(l, "e") && !strings.HasPrefix(l, "bureau"):
		return "la " + l
	}
	return "le " + l
}

// mediaURL turns a media_dir-relative path from the config into the URL the UI loads it from.
// The path is cleaned so a stray "../" in the config cannot point outside /media/.
func mediaURL(rel string) string {
	if rel == "" {
		return ""
	}
	rel = strings.TrimPrefix(path.Clean("/"+rel), "/")
	if rel == "" || rel == "." {
		return ""
	}
	return "/media/" + rel
}

// mediaExtras adds what needs the configuration to a speaker's state: the
// radio it plays (matched on the stream URL), the app and the album.
// preset: the house mode last chosen in Control while the zones still show it
// (night and boost leave the zones in manual; a boost lasts half an hour).
func preset(in Input, modes map[string]int) string {
	only := func(m string) bool { return len(modes) == 1 && modes[m] > 0 }
	switch in.HeatPreset {
	case "schedule":
		if only("schedule") {
			return "schedule"
		}
	case "off":
		if only("off") {
			return "off"
		}
	case "night":
		if modes["manual"] > 0 {
			return "night"
		}
	case "boost":
		if modes["manual"] > 0 && in.Now.Sub(in.HeatPresetAt) < 45*time.Minute {
			return "boost"
		}
	}
	if only("schedule") {
		return "schedule"
	}
	return ""
}

// ActiveMedia is the entity that holds what a speaker plays: its Music
// Assistant player (entities.music) while Music Assistant streams to it (the
// queue, the track, the artwork live there), else the speaker itself.
func ActiveMedia(dc config.Device, st map[string]ha.Entity) string {
	main, mus := dc.Entities["media_player"], dc.Entities["music"]
	m, ok := st[mus]
	if mus == "" || !ok || !available(m) {
		return main
	}
	c := st[main]
	if strings.Contains(strings.ToLower(str(c.Attributes["app_name"])), "music assistant") {
		return mus
	}
	busy := func(s string) bool { return s == "playing" || s == "paused" }
	if !busy(c.State) && busy(m.State) {
		return mus
	}
	return main
}

// mediaExtras: what a speaker plays (track, album, position, artwork, the
// radio it is on) and the controls it offers (previous, next, seek).
func mediaExtras(d *model.Device, cfg *config.Config, dc config.Device, st map[string]ha.Entity) {
	id := ActiveMedia(dc, st)
	e, ok := st[id]
	if !ok || e.Attributes == nil {
		return
	}
	if id != dc.Entities["media_player"] {
		switch e.State {
		case "playing", "paused":
			d.State.Playing = e.State
		case "buffering": // a seek, the next track loading
			d.State.Playing = "playing"
		case "idle", "on", "standby":
			if d.State.Playing != "off" {
				d.State.Playing = "idle"
			}
		}
		d.State.Title = str(e.Attributes["media_title"])
		d.State.Artist = str(e.Attributes["media_artist"])
		d.State.App = "Music Assistant"
	} else {
		d.State.App = str(e.Attributes["app_name"])
	}
	d.State.Album = str(e.Attributes["media_album_name"])
	uri := str(e.Attributes["media_content_id"])
	if len(uri) <= 400 {
		d.State.URI = uri
	}
	if uri != "" {
		base := func(u string) string {
			b, _, _ := strings.Cut(u, "?")
			return strings.TrimPrefix(strings.TrimPrefix(b, "https://"), "http://")
		}
		for _, r := range cfg.Media.Radios {
			if base(r.URL) == base(uri) {
				d.State.Radio = r.ID
				if d.State.Title == "" || d.State.Title == uri {
					d.State.Title = r.Name
				}
				if d.State.App == "" || strings.Contains(strings.ToLower(d.State.App), "receiver") {
					d.State.App = "Radio"
				}
				return // a live stream: no position, no queue
			}
		}
	}
	if dur := attrNum(e, "media_duration"); dur != nil && *dur > 0 {
		d.State.Duration = round0(dur)
		if pos := attrNum(e, "media_position"); pos != nil {
			d.State.Elapsed = round1(pos)
			d.State.ElapsedAt = str(e.Attributes["media_position_updated_at"])
		}
	}
	if pic := str(e.Attributes["entity_picture"]); strings.HasPrefix(pic, "https://i.scdn.co/") {
		d.State.Cover = pic
	} else if pic != "" {
		sum := sha1.Sum([]byte(pic))
		d.State.Cover = "/api/media/" + d.ID + "/cover?v=" + hex.EncodeToString(sum[:6])
	}
	if f := attrNum(e, "supported_features"); f != nil && d.State.Title != "" {
		feat := int(*f)
		if feat&16 != 0 {
			d.Capabilities = append(d.Capabilities, "previous")
		}
		if feat&32 != 0 {
			d.Capabilities = append(d.Capabilities, "next")
		}
		if feat&2 != 0 && d.State.Duration != nil {
			d.Capabilities = append(d.Capabilities, "seek")
		}
	}
}
