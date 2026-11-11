// Package mass searches and plays music through Music Assistant, via the
// Home Assistant actions music_assistant.search (with a response) and
// music_assistant.play_media. Music Assistant streams to the speaker itself
// (Cast included), so the speaker does not need to be visible in Spotify.
package mass

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"ahomelab.io/control/internal/model"
)

// HA is what this package needs from the Home Assistant client.
type HA interface {
	CallService(ctx context.Context, domain, service string, data map[string]any) error
	CallServiceResponse(ctx context.Context, domain, service string, data map[string]any) (json.RawMessage, error)
	ConfigEntryID(ctx context.Context, domain string) (string, error)
	BrowseMedia(ctx context.Context, entity, contentID, contentType string) (json.RawMessage, error)
}

type Client struct {
	HA HA
	// EntryID: the Music Assistant config entry; found on first use when empty.
	EntryID string

	mu sync.Mutex
}

func (c *Client) entry(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.EntryID != "" {
		return c.EntryID, nil
	}
	id, err := c.HA.ConfigEntryID(ctx, "music_assistant")
	if err != nil {
		return "", errors.New("Music Assistant n'est pas prêt dans Home Assistant : " + err.Error())
	}
	c.EntryID = id
	return id, nil
}

// Search returns up to limit tracks and limit/2 albums (every provider of
// Music Assistant: Spotify, library…).
func (c *Client) Search(ctx context.Context, q string, limit int) (model.Results, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return model.Results{Tracks: []model.Track{}, Albums: []model.Album{}}, nil
	}
	if limit <= 0 || limit > 25 {
		limit = 10
	}
	id, err := c.entry(ctx)
	if err != nil {
		return model.Results{}, err
	}
	raw, err := c.HA.CallServiceResponse(ctx, "music_assistant", "search", map[string]any{"config_entry_id": id, "name": q, "media_type": []string{"track", "album"}, "limit": limit})
	if err != nil {
		// the entry may have changed (reinstall): look it up again next time
		c.mu.Lock()
		c.EntryID = ""
		c.mu.Unlock()
		return model.Results{}, errors.New("recherche Music Assistant impossible : " + err.Error())
	}
	tracks, err := Tracks(raw, limit)
	if err != nil {
		return model.Results{}, err
	}
	albums, _ := Albums(raw, (limit+1)/2)
	return model.Results{Tracks: tracks, Albums: albums}, nil
}

// Play starts a track or an album (its Music Assistant URI) on a Music
// Assistant player entity, replacing the queue; then: tracks queued after a
// track (the rest of its album).
func (c *Client) Play(ctx context.Context, player, uri, kind string, then []string) error {
	if player == "" {
		return errors.New("pas de lecteur Music Assistant pour cette enceinte (entities.music dans control.yaml)")
	}
	if kind != "album" {
		kind = "track"
	}
	if !ValidURI(uri) {
		return errors.New("titre invalide")
	}
	var id any = uri
	if kind == "track" && len(then) > 0 {
		ids := []string{uri}
		for _, u := range then {
			if ValidURI(u) && len(ids) < 100 {
				ids = append(ids, u)
			}
		}
		id = ids
	}
	return c.HA.CallService(ctx, "music_assistant", "play_media", map[string]any{"entity_id": player, "media_id": id, "media_type": kind, "enqueue": "replace"})
}

// AlbumTracks lists an album's tracks through the media browser of a Music
// Assistant player (HA has no action for it).
func (c *Client) AlbumTracks(ctx context.Context, player, uri string, a model.Album) ([]model.Track, error) {
	if player == "" {
		return nil, errors.New("pas de lecteur Music Assistant (entities.music dans control.yaml)")
	}
	if !ValidURI(uri) {
		return nil, errors.New("album invalide")
	}
	raw, err := c.HA.BrowseMedia(ctx, player, uri, "album")
	if err != nil {
		return nil, errors.New("pistes de l'album introuvables : " + err.Error())
	}
	return BrowseTracks(raw, a.Artists)
}

// BrowseTracks reads the children of a browse_media answer. Music Assistant
// titles them "Artist - Title": the artist is split off.
func BrowseTracks(raw json.RawMessage, albumArtists string) ([]model.Track, error) {
	var res struct {
		Children []struct {
			Title      string `json:"title"`
			MediaClass string `json:"media_class"`
			ContentID  string `json:"media_content_id"`
			CanPlay    bool   `json:"can_play"`
		} `json:"children"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, errors.New("réponse de Music Assistant illisible")
	}
	type kid struct{ title, uri string }
	kids := []kid{}
	prefixed := 0
	for _, ch := range res.Children {
		if ch.Title == "" || !ValidURI(ch.ContentID) || (ch.MediaClass != "" && ch.MediaClass != "track" && ch.MediaClass != "music") {
			continue
		}
		if strings.Contains(ch.Title, " - ") {
			prefixed++
		}
		kids = append(kids, kid{ch.Title, ch.ContentID})
	}
	known := map[string]bool{}
	for _, a := range strings.Split(albumArtists, ",") {
		if a = strings.TrimSpace(strings.ToLower(a)); a != "" {
			known[a] = true
		}
	}
	all := len(kids) > 0 && prefixed == len(kids)
	out := []model.Track{}
	seen := map[string]bool{}
	for _, k := range kids {
		if seen[k.uri] {
			continue
		}
		seen[k.uri] = true
		t := model.Track{URI: k.uri, Title: k.title, Artists: albumArtists}
		if artist, title, ok := strings.Cut(k.title, " - "); ok && (all || known[strings.ToLower(artist)]) && title != "" {
			t.Title, t.Artists = title, artist
		}
		out = append(out, t)
	}
	return out, nil
}

// ValidURI: provider://type/id or provider:type:id, no spaces.
func ValidURI(u string) bool {
	if len(u) < 6 || len(u) > 300 || strings.ContainsAny(u, " \n\t\"'<>") {
		return false
	}
	return strings.Contains(u, "://") || strings.Count(u, ":") >= 2
}

type item struct {
	MediaType string  `json:"media_type"`
	URI       string  `json:"uri"`
	Name      string  `json:"name"`
	Version   string  `json:"version"`
	Image     *string `json:"image"`
	Duration  float64 `json:"duration"`
	Year      int     `json:"year"`
	Artists   []item  `json:"artists"`
	Album     *item   `json:"album"`
}

func artistNames(a []item) string {
	names := make([]string, 0, len(a))
	for _, x := range a {
		if x.Name != "" {
			names = append(names, x.Name)
		}
	}
	return strings.Join(names, ", ")
}

// cover keeps a public https image only (a local imageproxy URL would not load in the browser).
func cover(img *string) string {
	if img != nil && strings.HasPrefix(*img, "https://") && len(*img) <= 500 {
		return *img
	}
	return ""
}

// Albums reads the "albums" of a music_assistant.search response.
func Albums(raw json.RawMessage, limit int) ([]model.Album, error) {
	var res struct {
		Albums []item `json:"albums"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, errors.New("réponse de Music Assistant illisible")
	}
	out := []model.Album{}
	seen := map[string]bool{}
	for _, it := range res.Albums {
		if it.URI == "" || it.Name == "" || !ValidURI(it.URI) || seen[it.URI] {
			continue
		}
		seen[it.URI] = true
		title := it.Name
		if v := strings.TrimSpace(it.Version); v != "" && !strings.Contains(strings.ToLower(title), strings.ToLower(v)) {
			title += " (" + v + ")"
		}
		out = append(out, model.Album{URI: it.URI, Title: title, Artists: artistNames(it.Artists), Year: it.Year, Cover: cover(it.Image)})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// Tracks reads the "tracks" of a music_assistant.search response.
func Tracks(raw json.RawMessage, limit int) ([]model.Track, error) {
	var res struct {
		Tracks []item `json:"tracks"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, errors.New("réponse de Music Assistant illisible")
	}
	out := []model.Track{}
	seen := map[string]bool{}
	for _, it := range res.Tracks {
		if it.URI == "" || it.Name == "" || !ValidURI(it.URI) || seen[it.URI] {
			continue
		}
		seen[it.URI] = true
		title := it.Name
		if v := strings.TrimSpace(it.Version); v != "" && !strings.Contains(strings.ToLower(title), strings.ToLower(v)) {
			title += " (" + v + ")"
		}
		t := model.Track{URI: it.URI, Title: title, Artists: artistNames(it.Artists), Duration: int(it.Duration * 1000)}
		if it.Album != nil {
			t.Album = it.Album.Name
		}
		img := it.Image
		if (img == nil || *img == "") && it.Album != nil {
			img = it.Album.Image
		}
		t.Cover = cover(img)
		out = append(out, t)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
