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

// Search returns up to limit tracks (every provider of Music Assistant:
// Spotify, library…).
func (c *Client) Search(ctx context.Context, q string, limit int) ([]model.Track, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []model.Track{}, nil
	}
	if limit <= 0 || limit > 25 {
		limit = 10
	}
	id, err := c.entry(ctx)
	if err != nil {
		return nil, err
	}
	raw, err := c.HA.CallServiceResponse(ctx, "music_assistant", "search", map[string]any{"config_entry_id": id, "name": q, "media_type": []string{"track"}, "limit": limit})
	if err != nil {
		// the entry may have changed (reinstall): look it up again next time
		c.mu.Lock()
		c.EntryID = ""
		c.mu.Unlock()
		return nil, errors.New("recherche Music Assistant impossible : " + err.Error())
	}
	return Tracks(raw, limit)
}

// Play starts the track (its Music Assistant URI) on a Music Assistant player
// entity, replacing the queue.
func (c *Client) Play(ctx context.Context, player, uri string) error {
	if player == "" {
		return errors.New("pas de lecteur Music Assistant pour cette enceinte (entities.music dans control.yaml)")
	}
	if !validURI(uri) {
		return errors.New("titre invalide")
	}
	return c.HA.CallService(ctx, "music_assistant", "play_media", map[string]any{"entity_id": player, "media_id": uri, "media_type": "track", "enqueue": "replace"})
}

// validURI: provider://type/id or provider:type:id, no spaces.
func validURI(u string) bool {
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
	Artists   []item  `json:"artists"`
	Album     *item   `json:"album"`
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
		if it.URI == "" || it.Name == "" || !validURI(it.URI) || seen[it.URI] {
			continue
		}
		seen[it.URI] = true
		title := it.Name
		if v := strings.TrimSpace(it.Version); v != "" && !strings.Contains(strings.ToLower(title), strings.ToLower(v)) {
			title += " (" + v + ")"
		}
		names := make([]string, 0, len(it.Artists))
		for _, a := range it.Artists {
			if a.Name != "" {
				names = append(names, a.Name)
			}
		}
		t := model.Track{URI: it.URI, Title: title, Artists: strings.Join(names, ", "), Duration: int(it.Duration * 1000)}
		if it.Album != nil {
			t.Album = it.Album.Name
		}
		img := it.Image
		if (img == nil || *img == "") && it.Album != nil {
			img = it.Album.Image
		}
		if img != nil && strings.HasPrefix(*img, "https://") {
			t.Cover = *img
		}
		out = append(out, t)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
