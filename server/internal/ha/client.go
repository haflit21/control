package ha

// The WebSocket client to Home Assistant (/api/websocket): authenticates
// with the long-lived token of the "maison-app" user, loads every state
// once, then subscribes to state_changed and to the weather forecasts.
// Reconnects with a backoff; Connected() goes false meanwhile so the
// interface can show "connexion perdue".
//
// Auth model: one long-lived access token, read from HA_TOKEN by the
// caller; it never leaves this process. HA has no per-entity rights, so
// the server only calls the services of its allow list (internal/server).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"ahomelab.io/control/internal/httpx"
)

type Client struct {
	// DialTimeout bounds the WebSocket handshake (default 15 s).
	DialTimeout time.Duration
	*Store
	URL      string // http://192.168.10.242:8123
	Token    string
	Entities map[string]bool // configured entities; others are ignored
	Weather  string          // weather entity for forecast subscriptions
	Insecure bool            // TLS verification off (self-signed on the LAN)
	HTTP     *http.Client

	mu      sync.Mutex
	conn    *httpx.WSConn
	nextID  int
	pending map[int]chan json.RawMessage
}

func NewClient(url, token string, entities []string, weather string) *Client {
	set := map[string]bool{}
	for _, e := range entities {
		set[e] = true
	}
	return &Client{Store: NewStore(), URL: strings.TrimRight(url, "/"), Token: token, Entities: set, Weather: weather,
		HTTP: &http.Client{Timeout: 30 * time.Second}, pending: map[int]chan json.RawMessage{}}
}

// Run keeps the connection alive until ctx ends.
func (c *Client) Run(ctx context.Context) {
	backoff := time.Second
	for {
		started := time.Now()
		err := c.session(ctx)
		c.SetConnected(false)
		if ctx.Err() != nil {
			return
		}
		if time.Since(started) > time.Minute {
			backoff = time.Second // the connection had been fine: retry quickly
		}
		log.Printf("home assistant : déconnecté (%v) ; nouvel essai dans %s", err, backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

type wsMsg struct {
	ID      int             `json:"id,omitempty"`
	Type    string          `json:"type"`
	Success *bool           `json:"success,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Event   json.RawMessage `json:"event,omitempty"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

func (c *Client) session(ctx context.Context) error {
	wsURL := strings.Replace(strings.Replace(c.URL, "https://", "wss://", 1), "http://", "ws://", 1) + "/api/websocket"
	dt := c.DialTimeout
	if dt == 0 {
		dt = 15 * time.Second
	}
	dctx, cancel := context.WithTimeout(ctx, dt)
	conn, err := httpx.DialWS(dctx, wsURL, c.Insecure)
	cancel()
	if err != nil {
		return err
	}
	defer conn.Close()
	// when the session ends, forget the connection and fail the requests still waiting,
	// so callers wait for the next connection instead of writing to a closed socket
	defer func() {
		c.mu.Lock()
		if c.conn == conn {
			c.conn = nil
		}
		for id, ch := range c.pending {
			select {
			case ch <- json.RawMessage(`{"__error":"connexion à Home Assistant coupée"}`):
			default:
			}
			delete(c.pending, id)
		}
		c.mu.Unlock()
	}()
	var m wsMsg
	if err := conn.ReadJSON(&m); err != nil || m.Type != "auth_required" {
		return fmt.Errorf("auth_required attendu : %v %s", err, m.Type)
	}
	if err := conn.WriteJSON(map[string]string{"type": "auth", "access_token": c.Token}); err != nil {
		return err
	}
	if err := conn.ReadJSON(&m); err != nil {
		return err
	}
	if m.Type != "auth_ok" {
		return fmt.Errorf("authentification HA refusée : %s", m.Message)
	}
	c.mu.Lock()
	c.conn = conn
	c.nextID = 1
	c.pending = map[int]chan json.RawMessage{}
	c.mu.Unlock()

	// one reader: results go to the waiting request, events to the loop below
	readErr := make(chan error, 1)
	events := make(chan wsMsg, 256)
	go func() {
		for {
			var msg wsMsg
			if err := conn.ReadJSON(&msg); err != nil {
				readErr <- err
				return
			}
			if msg.Type == "result" || msg.Type == "pong" {
				c.mu.Lock()
				ch := c.pending[msg.ID]
				delete(c.pending, msg.ID)
				c.mu.Unlock()
				if ch != nil {
					if msg.Success != nil && !*msg.Success {
						emsg := "erreur HA"
						if msg.Error != nil {
							emsg = msg.Error.Code + " : " + msg.Error.Message
						}
						ch <- json.RawMessage(`{"__error":` + strconv(emsg) + `}`)
					} else if msg.Result == nil {
						ch <- json.RawMessage(`null`)
					} else {
						ch <- msg.Result
					}
				}
				continue
			}
			select {
			case events <- msg:
			default:
				log.Printf("home assistant : événements en retard, un message ignoré")
			}
		}
	}()

	// initial states, filtered to what control.yaml knows
	raw, err := c.request(ctx, map[string]any{"type": "get_states"})
	if err != nil {
		return fmt.Errorf("get_states : %w", err)
	}
	var all []Entity
	if err := json.Unmarshal(raw, &all); err != nil {
		return fmt.Errorf("get_states : %w", err)
	}
	keep := all[:0]
	for _, e := range all {
		if c.Entities[e.EntityID] {
			keep = append(keep, e)
		}
	}
	c.Replace(keep)
	subEvents, err := c.send(map[string]any{"type": "subscribe_events", "event_type": "state_changed"})
	if err != nil {
		return err
	}
	forecastIDs := map[int]string{}
	if c.Weather != "" {
		for _, kind := range []string{"hourly", "daily"} {
			id, err := c.send(map[string]any{"type": "weather/subscribe_forecast", "entity_id": c.Weather, "forecast_type": kind})
			if err != nil {
				return err
			}
			forecastIDs[id] = kind
		}
	}
	c.SetConnected(true)
	log.Printf("home assistant : connecté, %d entités suivies", len(keep))

	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readErr:
			return err
		case <-ping.C:
			go func() {
				pctx, cancel := context.WithTimeout(ctx, 20*time.Second)
				defer cancel()
				if _, err := c.request(pctx, map[string]any{"type": "ping"}); err != nil {
					conn.Close() // the reader returns, the session ends, Run reconnects
				}
			}()
		case msg := <-events:
			if msg.Type != "event" {
				continue
			}
			if msg.ID == subEvents {
				var ev struct {
					Data struct {
						EntityID string  `json:"entity_id"`
						NewState *Entity `json:"new_state"`
					} `json:"data"`
				}
				if err := json.Unmarshal(msg.Event, &ev); err == nil && ev.Data.NewState != nil && c.Entities[ev.Data.EntityID] {
					c.Set(*ev.Data.NewState)
				}
			} else if kind, ok := forecastIDs[msg.ID]; ok {
				var ev struct {
					Type     string           `json:"type"`
					Forecast []map[string]any `json:"forecast"`
				}
				if err := json.Unmarshal(msg.Event, &ev); err == nil && ev.Forecast != nil {
					c.SetForecast(c.Weather, kind, ev.Forecast)
				}
			}
		}
	}
}

func strconv(s string) string { b, _ := json.Marshal(s); return string(b) }

// send writes one command and returns its id (no wait).
func (c *Client) send(cmd map[string]any) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return 0, errors.New("home assistant : non connecté")
	}
	id := c.nextID
	c.nextID++
	cmd["id"] = id
	return id, c.conn.WriteJSON(cmd)
}

// request sends a command and waits for its result.
// errNotSent: the command never left (no connection, or the write failed) — safe to retry.
var errNotSent = errors.New("home assistant : non connecté")

func (c *Client) request(ctx context.Context, cmd map[string]any) (json.RawMessage, error) {
	ch := make(chan json.RawMessage, 1)
	c.mu.Lock()
	if c.conn == nil {
		c.mu.Unlock()
		return nil, errNotSent
	}
	id := c.nextID
	c.nextID++
	c.pending[id] = ch
	cmd["id"] = id
	err := c.conn.WriteJSON(cmd)
	if err != nil {
		delete(c.pending, id)
	}
	c.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("%w (%v)", errNotSent, err)
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(20 * time.Second):
		return nil, errors.New("home assistant : pas de réponse en 20 s")
	case res := <-ch:
		var e struct {
			Err string `json:"__error"`
		}
		if json.Unmarshal(res, &e) == nil && e.Err != "" {
			return nil, errors.New(e.Err)
		}
		return res, nil
	}
}

func (c *Client) CallService(ctx context.Context, domain, service string, data map[string]any) error {
	cmd := map[string]any{"type": "call_service", "domain": domain, "service": service}
	if target, ok := data["entity_id"]; ok {
		cmd["target"] = map[string]any{"entity_id": target}
		delete(data, "entity_id")
	}
	if len(data) > 0 {
		cmd["service_data"] = data
	}
	// during a reconnection (a few seconds), wait for HA rather than failing the tap;
	// a command that was never sent is retried once, one that was sent never is
	for attempt := 0; ; attempt++ {
		if err := c.waitConnected(ctx, 8*time.Second); err != nil {
			return err
		}
		_, err := c.request(ctx, cmd)
		if err == nil || !errors.Is(err, errNotSent) || attempt == 1 {
			return err
		}
	}
}

// waitConnected returns once the session is up (or after max / ctx).
func (c *Client) waitConnected(ctx context.Context, max time.Duration) error {
	deadline := time.Now().Add(max)
	for {
		c.mu.Lock()
		up := c.conn != nil
		c.mu.Unlock()
		if up && c.Connected() {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("home assistant injoignable (reconnexion en cours)")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
}

func (c *Client) rest(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.URL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	return c.HTTP.Do(req)
}

func (c *Client) CameraSnapshot(ctx context.Context, entity string) ([]byte, string, error) {
	resp, err := c.rest(ctx, "/api/camera_proxy/"+entity)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("caméra : HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return b, resp.Header.Get("Content-Type"), err
}

func (c *Client) CameraStream(ctx context.Context, entity string, w io.Writer, setHeader func(string)) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.URL+"/api/camera_proxy_stream/"+entity, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := (&http.Client{}).Do(req) // no timeout: the stream is long-lived
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("caméra : HTTP %d", resp.StatusCode)
	}
	setHeader(resp.Header.Get("Content-Type"))
	_, err = io.Copy(w, resp.Body)
	return err
}

// EntityIDs lists every entity HA knows (REST /api/states), for `control check`.
func (c *Client) EntityIDs(ctx context.Context) (map[string]string, error) {
	resp, err := c.rest(ctx, "/api/states")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("états : HTTP %d", resp.StatusCode)
	}
	var raw []struct {
		EntityID   string         `json:"entity_id"`
		Attributes map[string]any `json:"attributes"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&raw); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(raw))
	for _, s := range raw {
		name, _ := s.Attributes["friendly_name"].(string)
		out[s.EntityID] = name
	}
	return out, nil
}

// Services lists "domain.service" known to HA (REST /api/services), for `control check`.
func (c *Client) Services(ctx context.Context) (map[string]bool, error) {
	resp, err := c.rest(ctx, "/api/services")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("services : HTTP %d", resp.StatusCode)
	}
	var raw []struct {
		Domain   string                     `json:"domain"`
		Services map[string]json.RawMessage `json:"services"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&raw); err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, d := range raw {
		for s := range d.Services {
			out[d.Domain+"."+s] = true
		}
	}
	return out, nil
}

// CallServiceResponse runs a service that answers with data (return_response),
// e.g. music_assistant.search, and returns that data.
func (c *Client) CallServiceResponse(ctx context.Context, domain, service string, data map[string]any) (json.RawMessage, error) {
	cmd := map[string]any{"type": "call_service", "domain": domain, "service": service, "return_response": true}
	if target, ok := data["entity_id"]; ok {
		cmd["target"] = map[string]any{"entity_id": target}
		delete(data, "entity_id")
	}
	if len(data) > 0 {
		cmd["service_data"] = data
	}
	if err := c.waitConnected(ctx, 8*time.Second); err != nil {
		return nil, err
	}
	res, err := c.request(ctx, cmd)
	if err != nil {
		return nil, err
	}
	var r struct {
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(res, &r); err != nil {
		return nil, fmt.Errorf("réponse de %s.%s illisible", domain, service)
	}
	return r.Response, nil
}

// ConfigEntryID returns the id of the loaded config entry of an integration
// (the first one), e.g. the Music Assistant server.
func (c *Client) ConfigEntryID(ctx context.Context, domain string) (string, error) {
	if err := c.waitConnected(ctx, 8*time.Second); err != nil {
		return "", err
	}
	res, err := c.request(ctx, map[string]any{"type": "config_entries/get", "domain": domain})
	if err != nil {
		return "", err
	}
	var entries []struct {
		EntryID string `json:"entry_id"`
		Domain  string `json:"domain"`
		State   string `json:"state"`
	}
	if err := json.Unmarshal(res, &entries); err != nil {
		return "", fmt.Errorf("intégrations illisibles")
	}
	found := ""
	for _, e := range entries {
		if e.Domain != domain {
			continue
		}
		if e.State == "loaded" {
			return e.EntryID, nil
		}
		found = e.State
	}
	if found != "" {
		return "", fmt.Errorf("intégration %s présente mais pas chargée (%s)", domain, found)
	}
	return "", fmt.Errorf("intégration %s absente de Home Assistant", domain)
}

// StatesWith returns the entities whose attributes contain key (REST
// /api/states), e.g. the Music Assistant players (mass_player_type), for `control check`.
func (c *Client) StatesWith(ctx context.Context, key string) (map[string]string, error) {
	resp, err := c.rest(ctx, "/api/states")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("états : HTTP %d", resp.StatusCode)
	}
	var raw []struct {
		EntityID   string         `json:"entity_id"`
		Attributes map[string]any `json:"attributes"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&raw); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, s := range raw {
		if _, ok := s.Attributes[key]; ok {
			name, _ := s.Attributes["friendly_name"].(string)
			out[s.EntityID] = name
		}
	}
	return out, nil
}
