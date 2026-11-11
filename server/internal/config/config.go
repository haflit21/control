// Package config reads control.yaml: the rooms, the devices and the Home
// Assistant entities behind each of them. The file never holds a secret;
// HA_URL, HA_TOKEN… come from the environment, filled from the SOPS file
// .cloud/secrets/secrets.enc.yaml (or a .env in development).
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/goccy/go-yaml"

	"ahomelab.io/control/internal/model"
)

type Config struct {
	Name    string   `yaml:"name"`
	Rooms   []Room   `yaml:"rooms"`
	Globals Globals  `yaml:"globals"`
	Ignore  []string `yaml:"ignore"` // entities never shown (documentation only)
	Serve   Serve    `yaml:"serve"`
	Agenda  Agenda   `yaml:"agenda"`
}

// Agenda: the family calendar kept by the app itself (state_dir/agenda), and
// how a spoken or typed sentence becomes an event.
type Agenda struct {
	// Calendars group events by colour (Famille, Poubelles…). The first one is the default.
	Calendars []Calendar `yaml:"calendars"`
	Voice     Voice      `yaml:"voice"`
}

type Calendar struct {
	ID    string `yaml:"id"`
	Name  string `yaml:"name"`
	Color string `yaml:"color"` // CSS colour, optional
	// Hint helps the language model choose this calendar ("poubelles, tri, verre, encombrants").
	Hint string `yaml:"hint"`
}

// Voice: speech → text (the homelab's stt service, or any OpenAI-compatible
// /v1/audio/transcriptions endpoint: Whisper, speaches…), then
// text → event (a language model). Each part is optional: without `transcribe`
// there is no mic button, without `understand` no quick entry at all.
type Voice struct {
	Transcribe struct {
		URL      string `yaml:"url"`      // base URL, e.g. http://stt.home.arpa (…/v1/audio/transcriptions is appended)
		Model    string `yaml:"model"`    // e.g. parakeet-tdt-0.6b-v3 (stt), Systran/faster-whisper-small
		Language string `yaml:"language"` // default "fr"
	} `yaml:"transcribe"`
	Understand struct {
		// Provider: "gateway" (claude-gateway, the homelab's Claude service: the
		// owner's subscription, token in CLAUDE_GATEWAY_TOKEN), "anthropic"
		// (Messages API, key in ANTHROPIC_API_KEY) or "openai" (any
		// OpenAI-compatible chat endpoint: Ollama, llama.cpp, vLLM… key in LLM_API_KEY if needed).
		Provider string `yaml:"provider"`
		URL      string `yaml:"url"`     // gateway: http://claude.home.arpa ; openai: base URL ending in /v1 ; anthropic: optional
		Model    string `yaml:"model"`   // anthropic, openai: e.g. claude-haiku-5-5, qwen2.5:3b (gateway: chosen by its profile)
		Profile  string `yaml:"profile"` // gateway: default calendar
	} `yaml:"understand"`
}

// CalendarIDs lists the agenda calendar ids, default first.
func (a Agenda) CalendarIDs() []string {
	ids := make([]string, 0, len(a.Calendars))
	for _, c := range a.Calendars {
		ids = append(ids, c.ID)
	}
	return ids
}

type Serve struct {
	Listen    string   `yaml:"listen"`     // ":8080"
	AllowFrom []string `yaml:"allow_from"` // CIDRs, added to CONTROL_ALLOW_FROM
	StateDir  string   `yaml:"state_dir"`  // journal, default "state"
	MediaDir  string   `yaml:"media_dir"`  // photos served under /media/, default "media" (CONTROL_MEDIA overrides)
}

type Room struct {
	ID      string   `yaml:"id"`
	Name    string   `yaml:"name"`
	Icon    string   `yaml:"icon"`
	Area    string   `yaml:"area,omitempty"`  // HA area, documentation
	Image   string   `yaml:"image,omitempty"` // photo of the room, relative to media_dir (e.g. rooms/salon.jpg)
	Devices []Device `yaml:"devices"`
}

// Device maps one interface device to the HA entities it aggregates.
// Entities is role → entity_id; roles depend on the kind:
//
//	heating:  climate, humidity?, heating_power?, mold_risk?, battery?, zone_mode?, temperature?
//	ac:       climate, eco?, boost?, power?
//	light:    light  · light_group: light
//	cover:    cover  · plug: switch, power?
//	opening:  contact, battery?
//	camera:   camera, motion?, privacy?
//	media:    media_player
//	smoke:    smoke, co?
//	motion:   motion
//	climate_sensor: temperature?, humidity?
type Device struct {
	ID       string            `yaml:"id"`
	Kind     model.DeviceKind  `yaml:"kind"`
	Name     string            `yaml:"name"`
	Entities map[string]string `yaml:"entities"`
	Primary  bool              `yaml:"primary,omitempty"`
	Hidden   bool              `yaml:"hidden,omitempty"`
	Image    string            `yaml:"image,omitempty"`     // product photo, relative to media_dir; the UI has a default per kind
	ImageOff string            `yaml:"image_off,omitempty"` // lights: photo when off (image is then the lit one)
}

type Globals struct {
	Weather  string   `yaml:"weather"`
	Presence []Person `yaml:"presence"`
	Average  struct {
		Temperature string `yaml:"temperature"`
		Humidity    string `yaml:"humidity"`
	} `yaml:"average"`
	Heating struct {
		Schedule string `yaml:"schedule"`
		Night    string `yaml:"night"`
		Off      string `yaml:"off"`
		Boost    string `yaml:"boost"`
		Presence string `yaml:"presence"`
		Quota    string `yaml:"quota"`
	} `yaml:"heating"`
	Alarm struct {
		Armed string `yaml:"armed"`
		Ack   string `yaml:"ack"`
		Acked string `yaml:"acked"`
		Unifi string `yaml:"unifi"`
	} `yaml:"alarm"`
	Cameras struct {
		Private string `yaml:"private"` // binary_sensor: private mode active
		Toggle  string `yaml:"toggle"`  // script: toggle private mode
	} `yaml:"cameras"`
	Sun string `yaml:"sun"`
	// Air: outdoor air quality from Open-Meteo. Coordinates default to HA's zone.home.
	Air struct {
		Enabled   *bool   `yaml:"enabled"` // default true
		Latitude  float64 `yaml:"latitude"`
		Longitude float64 `yaml:"longitude"`
		Place     string  `yaml:"place"`
	} `yaml:"air"`
}

// AirEnabled says whether the air-quality poller runs (default yes).
func (g Globals) AirEnabled() bool { return g.Air.Enabled == nil || *g.Air.Enabled }

type Person struct {
	Entity string `yaml:"entity"`
	Name   string `yaml:"name"`
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%s : %w", path, err)
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("%s : %w", path, err)
	}
	return &c, nil
}

var kinds = map[model.DeviceKind]string{
	model.KindHeating: "climate", model.KindAC: "climate", model.KindLight: "light", model.KindLightGroup: "light",
	model.KindCover: "cover", model.KindPlug: "switch", model.KindOpening: "contact", model.KindCamera: "camera",
	model.KindMedia: "media_player", model.KindSmoke: "smoke", model.KindMotion: "motion", model.KindClimateSensor: "",
}

// Validate checks ids, kinds and the required entity role of each device.
func (c *Config) Validate() error {
	if c.Name == "" {
		c.Name = "Control"
	}
	if c.Serve.Listen == "" {
		c.Serve.Listen = ":8080"
	}
	if c.Serve.StateDir == "" {
		c.Serve.StateDir = "state"
	}
	if len(c.Agenda.Calendars) == 0 {
		c.Agenda.Calendars = []Calendar{{ID: "famille", Name: "Famille", Color: "#ec9b74"}}
	}
	calSeen := map[string]bool{}
	for i := range c.Agenda.Calendars {
		cal := &c.Agenda.Calendars[i]
		if cal.ID == "" || strings.ContainsAny(cal.ID, " ./@") {
			return fmt.Errorf("agenda : calendrier %q : id manquant ou invalide", cal.Name)
		}
		if calSeen[cal.ID] {
			return fmt.Errorf("agenda : calendrier %q en double", cal.ID)
		}
		calSeen[cal.ID] = true
		if cal.Name == "" {
			cal.Name = strings.ToUpper(cal.ID[:1]) + cal.ID[1:]
		}
	}
	v := &c.Agenda.Voice
	if v.Transcribe.Language == "" {
		v.Transcribe.Language = "fr"
	}
	switch v.Understand.Provider {
	case "", "anthropic", "openai":
	case "gateway":
		if v.Understand.URL == "" {
			v.Understand.URL = "http://claude.home.arpa"
		}
		if v.Understand.Profile == "" {
			v.Understand.Profile = "calendar"
		}
	default:
		return fmt.Errorf("agenda.voice.understand.provider : %q inconnu (gateway, anthropic ou openai)", v.Understand.Provider)
	}
	if v.Understand.Provider == "openai" && v.Understand.URL == "" {
		return fmt.Errorf("agenda.voice.understand.url est requis avec le fournisseur openai")
	}
	seen := map[string]bool{}
	for ri := range c.Rooms {
		r := &c.Rooms[ri]
		if r.ID == "" || strings.ContainsAny(r.ID, " ./") {
			return fmt.Errorf("pièce %q : id manquant ou invalide", r.Name)
		}
		if seen["room:"+r.ID] {
			return fmt.Errorf("pièce %q en double", r.ID)
		}
		seen["room:"+r.ID] = true
		if r.Name == "" {
			r.Name = strings.ToUpper(r.ID[:1]) + r.ID[1:]
		}
		if r.Icon == "" {
			r.Icon = "home"
		}
		for di := range r.Devices {
			d := &r.Devices[di]
			if d.ID == "" || strings.ContainsAny(d.ID, " ./") {
				return fmt.Errorf("pièce %s : appareil %q : id manquant ou invalide", r.ID, d.Name)
			}
			key := r.ID + "." + d.ID
			if seen[key] {
				return fmt.Errorf("appareil %s en double", key)
			}
			seen[key] = true
			req, ok := kinds[d.Kind]
			if !ok {
				return fmt.Errorf("%s : type %q inconnu", key, d.Kind)
			}
			if req != "" && d.Entities[req] == "" {
				return fmt.Errorf("%s : l'entité %q est obligatoire pour le type %s", key, req, d.Kind)
			}
			for role, e := range d.Entities {
				if !strings.Contains(e, ".") {
					return fmt.Errorf("%s : entité %s=%q invalide", key, role, e)
				}
			}
			if d.Name == "" {
				d.Name = d.ID
			}
		}
	}
	return nil
}

// Entities lists every HA entity the configuration refers to, so the HA
// subscription can be filtered to them.
func (c *Config) Entities() []string {
	set := map[string]bool{}
	add := func(e string) {
		if e != "" {
			set[e] = true
		}
	}
	for _, r := range c.Rooms {
		for _, d := range r.Devices {
			for _, e := range d.Entities {
				add(e)
			}
		}
	}
	g := c.Globals
	add(g.Weather)
	for _, p := range g.Presence {
		add(p.Entity)
	}
	add(g.Average.Temperature)
	add(g.Average.Humidity)
	add(g.Heating.Schedule)
	add(g.Heating.Night)
	add(g.Heating.Off)
	add(g.Heating.Boost)
	add(g.Heating.Presence)
	if g.AirEnabled() && (g.Air.Latitude == 0 || g.Air.Longitude == 0) {
		add("zone.home")
	}
	add(g.Heating.Quota)
	add(g.Alarm.Armed)
	add(g.Alarm.Ack)
	add(g.Alarm.Acked)
	add(g.Alarm.Unifi)
	add(g.Cameras.Private)
	add(g.Cameras.Toggle)
	add(g.Sun)
	out := make([]string, 0, len(set))
	for e := range set {
		out = append(out, e)
	}
	return out
}
