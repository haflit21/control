package build

import (
	"testing"

	"ahomelab.io/control/internal/config"
	"ahomelab.io/control/internal/ha"
	"ahomelab.io/control/internal/model"
)

// Music Assistant streaming to a Cast speaker: the track, the position and
// the artwork come from the MA player; the radio from the speaker itself.
func TestActiveMedia(t *testing.T) {
	cfg := &config.Config{}
	cfg.Media.Radios = []config.Radio{{ID: "inter", Name: "France Inter", URL: "https://icecast.radiofrance.fr/franceinter-midfi.mp3?id=radiofrance"}}
	dc := config.Device{ID: "enceinte", Kind: "media", Entities: map[string]string{"media_player": "media_player.salon", "music": "media_player.salon_salon"}}
	st := map[string]ha.Entity{
		"media_player.salon": {EntityID: "media_player.salon", State: "playing", Attributes: map[string]any{"app_name": "Music Assistant", "media_title": "flow"}},
		"media_player.salon_salon": {EntityID: "media_player.salon_salon", State: "paused", Attributes: map[string]any{
			"media_title": "Golden", "media_artist": "HUNTR/X", "media_album_name": "KPop Demon Hunters", "media_content_id": "spotify--uQXEcFjY://track/1CPZ",
			"media_duration": 194.0, "media_position": 12.34, "media_position_updated_at": "2026-11-11T19:38:20+00:00", "supported_features": 7796287.0,
			"entity_picture": "/api/media_player_proxy/media_player.salon_salon?token=x&cache=1"}},
	}
	if a := ActiveMedia(dc, st); a != "media_player.salon_salon" {
		t.Fatalf("actif : %s", a)
	}
	d := model.Device{ID: "salon.enceinte", Capabilities: []string{"play", "pause", "set_volume"}, State: model.State{Playing: "playing"}}
	mediaExtras(&d, cfg, dc, st)
	s := d.State
	if s.Playing != "paused" || s.Title != "Golden" || s.URI != "spotify--uQXEcFjY://track/1CPZ" || *s.Duration != 194 || *s.Elapsed != 12.3 || s.App != "Music Assistant" {
		t.Fatalf("état : %+v", s)
	}
	if len(s.Cover) < 30 || s.Cover[:30] != "/api/media/salon.enceinte/cove" {
		t.Fatalf("pochette : %q", s.Cover)
	}
	if len(d.Capabilities) != 6 {
		t.Fatalf("commandes : %v", d.Capabilities)
	}
	// a radio started by Control on the speaker: no queue, no position
	st["media_player.salon"] = ha.Entity{State: "playing", Attributes: map[string]any{"app_name": "Default Media Receiver", "media_title": "France Inter", "media_content_id": "https://icecast.radiofrance.fr/franceinter-midfi.mp3?id=x", "supported_features": 7796287.0}}
	st["media_player.salon_salon"] = ha.Entity{State: "idle", Attributes: map[string]any{}}
	d = model.Device{ID: "salon.enceinte", State: model.State{Playing: "playing", Title: "France Inter"}}
	mediaExtras(&d, cfg, dc, st)
	if d.State.Radio != "inter" || d.State.App != "Radio" || d.State.Duration != nil || len(d.Capabilities) != 0 {
		t.Fatalf("radio : %+v %v", d.State, d.Capabilities)
	}
}
