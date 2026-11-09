package build

import (
	"encoding/json"
	"testing"
	"time"

	"ahomelab.io/control/internal/config"
	"ahomelab.io/control/internal/demo"
)

func loadCfg(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load("../../../control.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestBuildFromDemo(t *testing.T) {
	cfg := loadCfg(t)
	src := demo.New(cfg)
	now := time.Now()
	h := Build(Input{Config: cfg, States: src.Snapshot(), Forecasts: src.Forecasts(cfg.Globals.Weather), Now: now, Connected: true, Demo: true})

	if len(h.Rooms) != 6 {
		t.Fatalf("6 pièces attendues, %d", len(h.Rooms))
	}
	salon := h.Rooms[0]
	if salon.ID != "salon" || salon.Summary.Temperature == nil || *salon.Summary.Temperature != 19.5 {
		t.Fatalf("salon : %+v", salon.Summary)
	}
	if !salon.Summary.Heating || salon.Summary.Target == nil || *salon.Summary.Target != 21 {
		t.Fatalf("salon doit chauffer vers 21° : %+v", salon.Summary)
	}
	// the hidden radiator head is not listed but its low battery is not lost either? it is hidden: no alert expected from it
	for _, d := range salon.Devices {
		if d.ID == "salon.tete" {
			t.Fatalf("un appareil caché ne doit pas être listé")
		}
	}
	cuisine := h.Rooms[1]
	if cuisine.Summary.OpeningsOpen != 1 || len(cuisine.Summary.Alerts) != 1 || cuisine.Summary.Alerts[0].Kind != "window_heating" {
		t.Fatalf("cuisine : fenêtre ouverte en chauffe attendue : %+v", cuisine.Summary)
	}
	if h.Globals.Heating.RoomsOn != 2 {
		t.Fatalf("2 pièces en chauffe attendues, %d", h.Globals.Heating.RoomsOn)
	}
	if h.Globals.Heating.Mode != "mixed" {
		t.Fatalf("mode global mixte attendu (schedule + off), %q", h.Globals.Heating.Mode)
	}
	if !h.Globals.Alarm.Armed || h.Globals.Alarm.UnifiState != "armed_away" {
		t.Fatalf("alarme : %+v", h.Globals.Alarm)
	}
	if h.Globals.LightsOn != 2 { // hue1 + hue2 (groups and hidden bulbs are not counted)
		t.Fatalf("2 lumières allumées attendues, %d", h.Globals.LightsOn)
	}
	if len(h.Presence) != 2 || !h.Presence[0].Home || h.Presence[1].Home {
		t.Fatalf("présence : %+v", h.Presence)
	}
	// unavailable devices older than an hour → one grouped alert
	var unav int
	for _, a := range h.Alerts {
		if a.Kind == "unavailable" {
			unav++
			if a.Title != "3 appareils injoignables" {
				t.Fatalf("titre injoignables : %q", a.Title)
			}
		}
	}
	if unav != 1 {
		t.Fatalf("une alerte injoignables attendue, %d", unav)
	}
	if h.Alerts[0].Kind != "window_heating" { // warn before info
		t.Fatalf("la fenêtre ouverte doit passer avant les injoignables : %+v", h.Alerts)
	}
	if h.Weather == nil || !h.Weather.Available || h.Weather.Condition != "cloudy" || len(h.Weather.Hourly) != 24 || len(h.Weather.Daily) != 7 {
		t.Fatalf("météo : %+v", h.Weather)
	}
	if h.Weather.Min == nil || *h.Weather.Min != 5 || h.Weather.Max == nil || *h.Weather.Max != 12 {
		t.Fatalf("min/max du jour : %v %v", h.Weather.Min, h.Weather.Max)
	}
	// every device id is "<room>.<device>" and JSON round-trips
	b, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if _, ok := back["rooms"]; !ok {
		t.Fatal("rooms manquant dans le JSON")
	}
}

func TestLightNormalisation(t *testing.T) {
	cfg := loadCfg(t)
	src := demo.New(cfg)
	st := src.Snapshot()
	h := Build(Input{Config: cfg, States: st, Now: time.Now(), Connected: true})
	var found bool
	for _, d := range h.Rooms[3].Devices { // bureau
		if d.ID == "bureau.hue1" {
			found = true
			if d.State.On == nil || !*d.State.On || d.State.Brightness == nil || *d.State.Brightness != 60 {
				t.Fatalf("hue1 : %+v", d.State)
			}
			if d.State.ColorTemp == nil || *d.State.ColorTemp != 2700 || d.State.Color == nil || d.State.Color.H != 30 {
				t.Fatalf("hue1 couleur : %+v", d.State)
			}
		}
		if d.ID == "bureau.store" && d.Available {
			t.Fatalf("le store doit être injoignable")
		}
		if d.ID == "bureau.volet" && (d.State.Position == nil || *d.State.Position != 60) {
			t.Fatalf("volet : %+v", d.State)
		}
	}
	if !found {
		t.Fatal("bureau.hue1 absent")
	}
}

func TestDisconnectedAlert(t *testing.T) {
	cfg := loadCfg(t)
	h := Build(Input{Config: cfg, States: nil, Now: time.Now(), Connected: false})
	var offline bool
	for _, a := range h.Alerts {
		if a.Kind == "offline" {
			offline = true
		}
	}
	if !offline {
		t.Fatal("alerte connexion perdue attendue")
	}
	if h.Weather == nil || h.Weather.Available {
		t.Fatalf("météo indisponible attendue : %+v", h.Weather)
	}
	for _, r := range h.Rooms {
		for _, d := range r.Devices {
			if d.Available {
				t.Fatalf("%s ne devrait pas être disponible sans état", d.ID)
			}
		}
	}
}
