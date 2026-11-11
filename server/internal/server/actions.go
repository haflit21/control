package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"ahomelab.io/control/internal/build"
	"ahomelab.io/control/internal/config"
	"ahomelab.io/control/internal/journal"
	"ahomelab.io/control/internal/model"
)

// Actions (étape 2). Every mutation is a POST with the X-Control header and the
// session cookie, translated into one or more Home Assistant service calls.
// The interface updates itself from the state_changed events that follow; the
// response only says whether HA accepted the call.

type actionBody struct {
	Action string          `json:"action"`
	Params json.RawMessage `json:"params"`
}

type call struct {
	domain, service string
	data            map[string]any
}

func (s *Server) decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(v); err != nil {
		fail(w, http.StatusBadRequest, "Requête illisible", err.Error())
		return false
	}
	return true
}

// run sends the calls to HA and writes the journal line (who / what / when).
func (s *Server) run(w http.ResponseWriter, r *http.Request, calls []call, okMsg string, je journal.Entry) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	je.Text = okMsg
	for _, c := range calls {
		if err := s.Src.CallService(ctx, c.domain, c.service, c.data); err != nil {
			je.Error = err.Error()
			s.record(r, je)
			fail(w, http.StatusBadGateway, "Home Assistant n'a pas accepté la commande", err.Error())
			return
		}
	}
	je.OK = true
	s.record(r, je)
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "message": okMsg})
}

// record appends to the journal; the user is the house's single user for now
// (the session model carries one label), the device comes from the User-Agent.
func (s *Server) record(r *http.Request, je journal.Entry) {
	if s.Journal == nil {
		return
	}
	je.User = s.userOf(r)
	je.Via = shortUA(r.UserAgent())
	s.Journal.Add(je)
}

// userOf names the session's user. One user in v1: the first person configured, else « Control ».
func (s *Server) userOf(_ *http.Request) string {
	if len(s.Cfg.Globals.Presence) > 0 && s.Cfg.Globals.Presence[0].Name != "" {
		return s.Cfg.Globals.Presence[0].Name
	}
	return "Control"
}

func (s *Server) findDevice(id string) (config.Room, config.Device, bool) {
	for _, r := range s.Cfg.Rooms {
		for _, d := range r.Devices {
			if r.ID+"."+d.ID == id {
				return r, d, true
			}
		}
	}
	return config.Room{}, config.Device{}, false
}

func target(entity string, extra map[string]any) map[string]any {
	data := map[string]any{"entity_id": entity}
	for k, v := range extra {
		data[k] = v
	}
	return data
}

func onOff(on bool) string {
	if on {
		return "turn_on"
	}
	return "turn_off"
}

// deviceAction: POST /api/devices/{id}/actions {action, params}
func (s *Server) deviceAction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rc, dc, ok := s.findDevice(id)
	if !ok {
		fail(w, http.StatusNotFound, "Appareil inconnu", id)
		return
	}
	var body actionBody
	if !s.decode(w, r, &body) {
		return
	}
	if dc.Kind == model.KindMedia && (body.Action == "play_music" || body.Action == "play_track") {
		s.playMusic(w, r, id, rc, dc, body.Action, body.Params)
		return
	}
	calls, msg, err := s.planDevice(dc, body)
	if err != nil {
		s.record(r, journal.Entry{Scope: "device", Target: id, Action: body.Action, Params: journal.Compact(body.Params), Text: err.Error()})
		fail(w, http.StatusBadRequest, err.Error(), "")
		return
	}
	s.run(w, r, calls, msg, journal.Entry{Scope: "device", Target: id, Action: body.Action, Params: journal.Compact(body.Params)})
}

// planDevice maps an action to HA service calls for a device of this kind.
func (s *Server) planDevice(dc config.Device, body actionBody) ([]call, string, error) {
	p := func(v any) error {
		if len(body.Params) == 0 {
			return fmt.Errorf("paramètres manquants")
		}
		return json.Unmarshal(body.Params, v)
	}
	ent := dc.Entities
	switch dc.Kind {
	case model.KindLight, model.KindLightGroup:
		e := ent["light"]
		switch body.Action {
		case "power":
			var q struct{ On bool }
			if err := p(&q); err != nil {
				return nil, "", err
			}
			return []call{{"light", onOff(q.On), target(e, nil)}}, dc.Name + " · " + map[bool]string{true: "allumée", false: "éteinte"}[q.On], nil
		case "set_brightness":
			var q struct{ Pct float64 }
			if err := p(&q); err != nil {
				return nil, "", err
			}
			if q.Pct <= 0 {
				return []call{{"light", "turn_off", target(e, nil)}}, dc.Name + " · éteinte", nil
			}
			return []call{{"light", "turn_on", target(e, map[string]any{"brightness_pct": clamp(q.Pct, 1, 100)})}}, fmt.Sprintf("%s · %.0f %%", dc.Name, q.Pct), nil
		case "set_color_temp":
			var q struct{ Kelvin float64 }
			if err := p(&q); err != nil {
				return nil, "", err
			}
			return []call{{"light", "turn_on", target(e, map[string]any{"color_temp_kelvin": clamp(q.Kelvin, 2000, 6500)})}}, fmt.Sprintf("%s · %.0f K", dc.Name, q.Kelvin), nil
		case "set_color":
			var q struct {
				HS struct{ H, S float64 } `json:"hs"`
			}
			if err := p(&q); err != nil {
				return nil, "", err
			}
			return []call{{"light", "turn_on", target(e, map[string]any{"hs_color": []float64{clamp(q.HS.H, 0, 360), clamp(q.HS.S, 0, 100)}})}}, dc.Name + " · couleur", nil
		}
	case model.KindPlug:
		if body.Action == "power" {
			var q struct{ On bool }
			if err := p(&q); err != nil {
				return nil, "", err
			}
			e := ent["switch"]
			domain := strings.SplitN(e, ".", 2)[0]
			if domain != "switch" && domain != "light" && domain != "input_boolean" {
				domain = "homeassistant"
			}
			return []call{{domain, onOff(q.On), target(e, nil)}}, dc.Name + " · " + map[bool]string{true: "allumée", false: "éteinte"}[q.On], nil
		}
	case model.KindCover:
		e := ent["cover"]
		switch body.Action {
		case "open":
			return []call{{"cover", "open_cover", target(e, nil)}}, dc.Name + " · ouverture", nil
		case "close":
			return []call{{"cover", "close_cover", target(e, nil)}}, dc.Name + " · fermeture", nil
		case "stop":
			return []call{{"cover", "stop_cover", target(e, nil)}}, dc.Name + " · stop", nil
		case "set_position":
			var q struct{ Pct float64 }
			if err := p(&q); err != nil {
				return nil, "", err
			}
			return []call{{"cover", "set_cover_position", target(e, map[string]any{"position": int(clamp(q.Pct, 0, 100))})}}, fmt.Sprintf("%s · %.0f %%", dc.Name, q.Pct), nil
		}
	case model.KindHeating:
		e := ent["climate"]
		switch body.Action {
		case "set_target":
			var q struct{ Temperature float64 }
			if err := p(&q); err != nil {
				return nil, "", err
			}
			t := clamp(q.Temperature, 5, 25)
			return []call{{"climate", "set_temperature", target(e, map[string]any{"temperature": t, "hvac_mode": "heat"})}}, fmt.Sprintf("%s · consigne %s°", dc.Name, trim(t)), nil
		case "set_mode":
			var q struct {
				Mode     string          `json:"mode"`
				Duration json.RawMessage `json:"duration"`
			}
			if err := p(&q); err != nil {
				return nil, "", err
			}
			switch q.Mode {
			case "off":
				return []call{{"climate", "set_hvac_mode", target(e, map[string]any{"hvac_mode": "off"})}}, dc.Name + " · arrêt", nil
			case "schedule":
				return []call{{"climate", "set_hvac_mode", target(e, map[string]any{"hvac_mode": "auto"})}}, dc.Name + " · planning", nil
			case "manual":
				// Tado through Tado Hijack (the zone's zone_mode sensor): an overlay
				// with a termination. Other climates just switch to heat.
				data := map[string]any{"hvac_mode": "heat"}
				if cur, ok := s.currentTarget(e); ok {
					data["temperature"] = cur
				}
				if zone := ent["zone_mode"]; zone != "" {
					hd := map[string]any{"entity_id": zone, "hvac_mode": "heat", "overlay": "next_block", "refresh_after": true}
					if t, ok := data["temperature"]; ok {
						hd["temperature"] = t
					}
					switch string(q.Duration) {
					case `"next_block"`, "":
					case `"forever"`:
						hd["overlay"] = "manual"
					default:
						var mins int
						if err := json.Unmarshal(q.Duration, &mins); err == nil && mins > 0 {
							hd["overlay"] = "manual"
							hd["duration"] = min(mins, 1440)
						}
					}
					return []call{{"tado_hijack", "set_mode", hd}}, dc.Name + " · manuel", nil
				}
				return []call{{"climate", "set_temperature", target(e, data)}}, dc.Name + " · manuel", nil
			}
			return nil, "", fmt.Errorf("mode inconnu : %q", q.Mode)
		}
	case model.KindAC:
		e := ent["climate"]
		switch body.Action {
		case "power":
			var q struct{ On bool }
			if err := p(&q); err != nil {
				return nil, "", err
			}
			if q.On {
				return []call{{"climate", "set_hvac_mode", target(e, map[string]any{"hvac_mode": "cool"})}}, dc.Name + " · allumée", nil
			}
			return []call{{"climate", "set_hvac_mode", target(e, map[string]any{"hvac_mode": "off"})}}, dc.Name + " · éteinte", nil
		case "set_mode":
			var q struct{ Mode string }
			if err := p(&q); err != nil {
				return nil, "", err
			}
			return []call{{"climate", "set_hvac_mode", target(e, map[string]any{"hvac_mode": q.Mode})}}, dc.Name + " · " + q.Mode, nil
		case "set_target":
			var q struct{ Temperature float64 }
			if err := p(&q); err != nil {
				return nil, "", err
			}
			t := clamp(q.Temperature, 16, 30)
			return []call{{"climate", "set_temperature", target(e, map[string]any{"temperature": t})}}, fmt.Sprintf("%s · %s°", dc.Name, trim(t)), nil
		case "set_eco", "set_boost":
			var q struct{ On bool }
			if err := p(&q); err != nil {
				return nil, "", err
			}
			role := strings.TrimPrefix(body.Action, "set_")
			sw := ent[role]
			if sw == "" {
				return nil, "", fmt.Errorf("pas d'entité %s pour %s", role, dc.Name)
			}
			return []call{{"switch", onOff(q.On), target(sw, nil)}}, dc.Name + " · " + role, nil
		}
	case model.KindMedia:
		e := ent["media_player"]
		snap := s.Src.Snapshot()
		// what plays: the Music Assistant player while it streams to the speaker
		act := build.ActiveMedia(dc, snap)
		switch body.Action {
		case "play":
			// A Cast speaker can only resume a paused session; idle or off,
			// media_play fails ("Failed to execute play"). Start the last radio instead.
			if st, ok := snap[act]; ok && st.State != "paused" && st.State != "playing" {
				rd, ok := s.resumeRadio(e, snap[e])
				if !ok {
					return nil, "", fmt.Errorf("rien à reprendre sur %s : choisir une radio ou un titre", dc.Name)
				}
				s.lastRadio.Store(e, rd.ID)
				return []call{radioCall(e, rd)}, dc.Name + " · " + rd.Name, nil
			}
			return []call{{"media_player", "media_play", target(act, nil)}}, dc.Name + " · lecture", nil
		case "pause":
			return []call{{"media_player", "media_pause", target(act, nil)}}, dc.Name + " · pause", nil
		case "next":
			return []call{{"media_player", "media_next_track", target(act, nil)}}, dc.Name + " · titre suivant", nil
		case "previous":
			return []call{{"media_player", "media_previous_track", target(act, nil)}}, dc.Name + " · titre précédent", nil
		case "seek":
			var q struct{ Position float64 }
			if err := p(&q); err != nil {
				return nil, "", err
			}
			if q.Position < 0 || q.Position > 24*3600 {
				return nil, "", fmt.Errorf("position hors limites")
			}
			return []call{{"media_player", "media_seek", target(act, map[string]any{"seek_position": math.Round(q.Position)})}}, fmt.Sprintf("%s · %s", dc.Name, minSec(q.Position)), nil
		case "set_volume":
			var q struct{ Pct float64 }
			if err := p(&q); err != nil {
				return nil, "", err
			}
			return []call{{"media_player", "volume_set", target(e, map[string]any{"volume_level": clamp(q.Pct, 0, 100) / 100})}}, fmt.Sprintf("%s · volume %.0f %%", dc.Name, q.Pct), nil
		case "play_radio":
			var q struct{ ID string }
			if err := p(&q); err != nil {
				return nil, "", err
			}
			rd, ok := s.Cfg.Radio(q.ID)
			if !ok {
				return nil, "", fmt.Errorf("radio inconnue : %q", q.ID)
			}
			s.lastRadio.Store(e, rd.ID)
			return []call{radioCall(e, rd)}, dc.Name + " · " + rd.Name, nil
		}
	case model.KindCamera:
		if body.Action == "set_privacy" {
			var q struct{ On bool }
			if err := p(&q); err != nil {
				return nil, "", err
			}
			return s.privacyCalls(q.On)
		}
	}
	return nil, "", fmt.Errorf("action %q impossible sur %s", body.Action, dc.Name)
}

func minSec(sec float64) string {
	n := int(math.Round(sec))
	return fmt.Sprintf("%d:%02d", n/60, n%60)
}

// privacyCalls toggles the cameras' private mode via the configured script
// (the script toggles, so only call it when the state differs).
func (s *Server) privacyCalls(on bool) ([]call, string, error) {
	g := s.Cfg.Globals.Cameras
	if g.Toggle == "" {
		return nil, "", fmt.Errorf("pas de script de mode privé configuré")
	}
	cur := false
	if e, ok := s.Src.Snapshot()[g.Private]; ok {
		cur = e.State == "on"
	}
	msg := map[bool]string{true: "Mode privé activé", false: "Mode privé désactivé"}[on]
	if cur == on {
		return []call{}, msg, nil
	}
	domain := strings.SplitN(g.Toggle, ".", 2)[0]
	return []call{{domain, "turn_on", target(g.Toggle, nil)}}, msg, nil
}

func (s *Server) currentTarget(entity string) (float64, bool) {
	e, ok := s.Src.Snapshot()[entity]
	if !ok {
		return 0, false
	}
	t, ok := e.Attributes["temperature"].(float64)
	return t, ok
}

// houseHeating: POST /api/home/heating {mode: schedule|night|off|boost}
func (s *Server) houseHeating(w http.ResponseWriter, r *http.Request) {
	var q struct{ Mode string }
	if !s.decode(w, r, &q) {
		return
	}
	g := s.Cfg.Globals.Heating
	var e, msg string
	switch q.Mode {
	case "schedule":
		e, msg = g.Schedule, "Chauffage · planning"
	case "night":
		e, msg = g.Night, "Chauffage · nuit"
	case "off":
		e, msg = g.Off, "Chauffage coupé"
	case "boost":
		e, msg = g.Boost, "Boost lancé"
	default:
		fail(w, http.StatusBadRequest, "Mode inconnu", q.Mode)
		return
	}
	if e == "" {
		fail(w, http.StatusBadRequest, "Mode non configuré", q.Mode)
		return
	}
	domain := strings.SplitN(e, ".", 2)[0]
	service := "turn_on"
	if domain == "button" || domain == "input_button" {
		service = "press"
	}
	s.run(w, r, []call{{domain, service, target(e, nil)}}, msg, journal.Entry{Scope: "house", Action: "heating", Params: journal.Compact(map[string]string{"mode": q.Mode})})
}

// lightsOff: POST /api/home/lights/off — every configured light.
func (s *Server) lightsOff(w http.ResponseWriter, r *http.Request) {
	var ids []string
	for _, rc := range s.Cfg.Rooms {
		for _, d := range rc.Devices {
			if (d.Kind == model.KindLight || d.Kind == model.KindLightGroup) && d.Entities["light"] != "" {
				ids = append(ids, d.Entities["light"])
			}
		}
	}
	if len(ids) == 0 {
		fail(w, http.StatusBadRequest, "Aucune lumière configurée", "")
		return
	}
	s.run(w, r, []call{{"light", "turn_off", map[string]any{"entity_id": ids}}}, "Toutes les lumières éteintes", journal.Entry{Scope: "house", Action: "lights_off"})
}

// alarm: POST /api/security/alarm {armed: bool}
func (s *Server) alarm(w http.ResponseWriter, r *http.Request) {
	var q struct{ Armed bool }
	if !s.decode(w, r, &q) {
		return
	}
	e := s.Cfg.Globals.Alarm.Armed
	if e == "" {
		fail(w, http.StatusBadRequest, "Alarme non configurée", "")
		return
	}
	domain := strings.SplitN(e, ".", 2)[0]
	s.run(w, r, []call{{domain, onOff(q.Armed), target(e, nil)}}, map[bool]string{true: "Alarme armée", false: "Alarme désarmée"}[q.Armed], journal.Entry{Scope: "security", Action: "alarm", Params: journal.Compact(map[string]bool{"armed": q.Armed})})
}

// alarmAck: POST /api/security/alarm/ack
func (s *Server) alarmAck(w http.ResponseWriter, r *http.Request) {
	e := s.Cfg.Globals.Alarm.Ack
	if e == "" {
		fail(w, http.StatusBadRequest, "Acquittement non configuré", "")
		return
	}
	domain := strings.SplitN(e, ".", 2)[0]
	s.run(w, r, []call{{domain, "press", target(e, nil)}}, "Alerte vue", journal.Entry{Scope: "security", Action: "alarm_ack"})
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func trim(f float64) string {
	s := fmt.Sprintf("%.1f", f)
	s = strings.TrimSuffix(s, ".0")
	return strings.Replace(s, ".", ",", 1)
}
