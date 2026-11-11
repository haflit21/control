package mass

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"ahomelab.io/control/internal/model"
)

type fakeHA struct {
	entry    string
	calls    []map[string]any
	response string
	fail     error
}

func (f *fakeHA) CallService(ctx context.Context, domain, service string, data map[string]any) error {
	f.calls = append(f.calls, map[string]any{"svc": domain + "." + service, "data": data})
	return f.fail
}

func (f *fakeHA) CallServiceResponse(ctx context.Context, domain, service string, data map[string]any) (json.RawMessage, error) {
	f.calls = append(f.calls, map[string]any{"svc": domain + "." + service, "data": data})
	if f.fail != nil {
		return nil, f.fail
	}
	return json.RawMessage(f.response), nil
}

func (f *fakeHA) BrowseMedia(ctx context.Context, entity, contentID, contentType string) (json.RawMessage, error) {
	f.calls = append(f.calls, map[string]any{"svc": "browse", "data": map[string]any{"entity_id": entity, "id": contentID, "type": contentType}})
	return json.RawMessage(`{"title":"KPop Demon Hunters","children":[
	 {"title":"HUNTR/X - Golden","media_class":"track","media_content_id":"spotify://track/1","can_play":true},
	 {"title":"Saja Boys - Soda Pop","media_class":"track","media_content_id":"spotify://track/2","can_play":true},
	 {"title":"HUNTR/X - Golden","media_class":"track","media_content_id":"spotify://track/1"},
	 {"title":"cassé","media_class":"track","media_content_id":"bad uri"}]}`), nil
}

func (f *fakeHA) ConfigEntryID(ctx context.Context, domain string) (string, error) {
	if f.entry == "" {
		return "", errors.New("intégration music_assistant absente de Home Assistant")
	}
	return f.entry, nil
}

const golden = `{"artists":[],"albums":[],"tracks":[
 {"media_type":"track","uri":"spotify://track/1","name":"Golden","version":"","image":"https://i.scdn.co/image/a","duration":195,
  "artists":[{"media_type":"artist","uri":"spotify://artist/h","name":"HUNTR/X","version":"","image":null},{"name":"EJAE"}],
  "album":{"media_type":"album","uri":"spotify://album/k","name":"KPop Demon Hunters","version":"","image":"https://i.scdn.co/image/b"}},
 {"media_type":"track","uri":"spotify://track/1","name":"Golden","artists":[]},
 {"media_type":"track","uri":"library://track/9","name":"Golden Brown","version":"Remastered","image":"http://192.168.10.242:8095/imageproxy?x",
  "artists":[{"name":"The Stranglers"}],"album":{"name":"La Folie","image":"https://i.scdn.co/image/c"}},
 {"media_type":"track","uri":"bad uri","name":"Cassé"}
],"albums":[
 {"media_type":"album","uri":"spotify://album/k","name":"KPop Demon Hunters","version":"","year":2025,"image":"https://i.scdn.co/image/b","artists":[{"name":"Various Artists"}]},
 {"media_type":"album","uri":"spotify://album/k","name":"KPop Demon Hunters"},
 {"media_type":"album","uri":"library://album/3","name":"La Folie","image":"http://192.168.10.242:8095/imageproxy?y","artists":[{"name":"The Stranglers"}]}
]}`

func TestSearchAndPlay(t *testing.T) {
	f := &fakeHA{entry: "e1", response: golden}
	c := &Client{HA: f}
	ctx := context.Background()
	res, err := c.Search(ctx, " golden ", 5)
	if err != nil {
		t.Fatal(err)
	}
	tr := res.Tracks
	if len(res.Albums) != 2 || res.Albums[0].Year != 2025 || res.Albums[0].Artists != "Various Artists" || res.Albums[0].Cover != "https://i.scdn.co/image/b" || res.Albums[1].Cover != "" {
		t.Fatalf("albums : %+v", res.Albums)
	}
	if len(tr) != 2 {
		t.Fatalf("doublon ou URI invalide gardé : %+v", tr)
	}
	if tr[0].Title != "Golden" || tr[0].Artists != "HUNTR/X, EJAE" || tr[0].Album != "KPop Demon Hunters" || tr[0].Duration != 195000 || tr[0].Cover != "https://i.scdn.co/image/a" {
		t.Fatalf("premier titre : %+v", tr[0])
	}
	if tr[1].Title != "Golden Brown (Remastered)" || tr[1].Cover != "" {
		t.Fatalf("version, ou image locale (http) gardée : %+v", tr[1])
	}
	d := f.calls[0]["data"].(map[string]any)
	if f.calls[0]["svc"] != "music_assistant.search" || d["config_entry_id"] != "e1" || d["name"] != "golden" || d["limit"] != 5 || len(d["media_type"].([]string)) != 2 {
		t.Fatalf("appel : %+v", f.calls[0])
	}
	if err := c.Play(ctx, "media_player.salon_2", tr[0].URI, "track", nil); err != nil {
		t.Fatal(err)
	}
	p := f.calls[1]["data"].(map[string]any)
	if f.calls[1]["svc"] != "music_assistant.play_media" || p["entity_id"] != "media_player.salon_2" || p["media_id"] != "spotify://track/1" || p["enqueue"] != "replace" {
		t.Fatalf("lecture : %+v", f.calls[1])
	}
	if err := c.Play(ctx, "media_player.salon_2", res.Albums[0].URI, "album", nil); err != nil || f.calls[2]["data"].(map[string]any)["media_type"] != "album" {
		t.Fatalf("album : %v %+v", err, f.calls[2])
	}
	if err := c.Play(ctx, "", tr[0].URI, "track", nil); err == nil || !strings.Contains(err.Error(), "entities.music") {
		t.Fatalf("lecteur manquant : %v", err)
	}
	if err := c.Play(ctx, "media_player.x", "javascript:alert(1)", "track", nil); err == nil {
		t.Fatal("URI invalide acceptée")
	}
}

func TestAlbumTracks(t *testing.T) {
	f := &fakeHA{entry: "e1"}
	c := &Client{HA: f}
	ctx := context.Background()
	tr, err := c.AlbumTracks(ctx, "media_player.salon_salon", "spotify://album/k", model.Album{Artists: "Various Artists"})
	if err != nil || len(tr) != 2 || tr[0].Title != "Golden" || tr[0].Artists != "HUNTR/X" || tr[1].Title != "Soda Pop" {
		t.Fatalf("pistes : %v %+v", err, tr)
	}
	// a track and the rest of its album: one play_media with the list
	if err := c.Play(ctx, "media_player.salon_salon", tr[0].URI, "track", []string{tr[1].URI, "bad uri"}); err != nil {
		t.Fatal(err)
	}
	ids, _ := f.calls[1]["data"].(map[string]any)["media_id"].([]string)
	if len(ids) != 2 || ids[1] != "spotify://track/2" {
		t.Fatalf("file : %+v", f.calls[1])
	}
	// titles without the artist prefix stay whole
	tr, _ = BrowseTracks(json.RawMessage(`{"children":[{"title":"Get Lucky - Radio Edit","media_class":"track","media_content_id":"spotify://track/9"},{"title":"Lose Yourself","media_class":"track","media_content_id":"spotify://track/8"}]}`), "Daft Punk")
	if tr[0].Title != "Get Lucky - Radio Edit" || tr[0].Artists != "Daft Punk" {
		t.Fatalf("titre avec tiret : %+v", tr)
	}
}

func TestNotInstalled(t *testing.T) {
	c := &Client{HA: &fakeHA{}}
	if _, err := c.Search(context.Background(), "x", 5); err == nil || !strings.Contains(err.Error(), "pas prêt") {
		t.Fatalf("absent : %v", err)
	}
}
