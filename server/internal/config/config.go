// Package config reads maison.yaml: the rooms, the devices and the Home
// Assistant entities behind each of them. The file never holds a secret;
// HA_URL and HA_TOKEN come from the environment (.env).
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/goccy/go-yaml"

	"ahomelab.io/maison/internal/model"
)

type Config struct {
	Name    string   `yaml:"name"`
	Rooms   []Room   `yaml:"rooms"`
	Globals Globals  `yaml:"globals"`
	Ignore  []string `yaml:"ignore"` // entities never shown (documentation only)
	Serve   Serve    `yaml:"serve"`
}

type Serve struct {
	Listen    string   `yaml:"listen"`     // ":8080"
	AllowFrom []string `yaml:"allow_from"` // CIDRs, added to MAISON_ALLOW_FROM
	StateDir  string   `yaml:"state_dir"`  // journal, default "state"
	MediaDir  string   `yaml:"media_dir"`  // photos served under /media/, default "media" (MAISON_MEDIA overrides)
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
	// Calendars: HA calendar entities shown in the Agenda tab.
	Calendars []CalendarConfig `yaml:"calendars"`
}

type CalendarConfig struct {
	Entity string `yaml:"entity"`
	Name   string `yaml:"name"`
	Color  string `yaml:"color"` // CSS colour, optional
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
		c.Name = "Maison"
	}
	if c.Serve.Listen == "" {
		c.Serve.Listen = ":8080"
	}
	if c.Serve.StateDir == "" {
		c.Serve.StateDir = "state"
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
	for _, cal := range g.Calendars {
		add(cal.Entity)
	}
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
