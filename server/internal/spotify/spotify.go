// Package spotify is the small part of the Spotify Web API that Control needs:
// search tracks, find the house speaker among the Spotify Connect devices and
// start a track on it. Authorization code flow: the refresh token is obtained
// once with `control spotify-login` and kept in the encrypted secrets.
//
// Spotify's rules (2026): the app owner needs Spotify Premium, a development
// mode app serves at most five allowlisted users, the redirect URI must be a
// loopback IP (http://127.0.0.1:8888/callback), never "localhost".
package spotify

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"

	"ahomelab.io/control/internal/model"
)

const (
	DefaultAPI      = "https://api.spotify.com/v1"
	DefaultAccounts = "https://accounts.spotify.com"
	RedirectURI     = "http://127.0.0.1:8888/callback"
	Scopes          = "user-read-playback-state user-modify-playback-state"
)

// Client talks to the Web API with an access token refreshed when needed.
type Client struct {
	ClientID, ClientSecret, RefreshToken string
	Market                               string // search market, e.g. FR
	API, Accounts                        string // base URLs (tests)
	HTTP                                 *http.Client
	// OnRefreshToken is called when Spotify hands out a new refresh token.
	OnRefreshToken func(string)

	mu     sync.Mutex
	access string
	exp    time.Time
}

// Error is a Spotify refusal, with a French message for the interface.
type Error struct {
	Status  int
	Reason  string // Spotify's reason (NO_ACTIVE_DEVICE, PREMIUM_REQUIRED…)
	Message string
}

func (e *Error) Error() string { return e.Message }

func (c *Client) api() string {
	if c.API != "" {
		return strings.TrimRight(c.API, "/")
	}
	return DefaultAPI
}

func (c *Client) accounts() string {
	if c.Accounts != "" {
		return strings.TrimRight(c.Accounts, "/")
	}
	return DefaultAccounts
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second}
}

// token returns a valid access token, refreshing it a minute before it expires.
func (c *Client) token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.access != "" && time.Now().Before(c.exp.Add(-time.Minute)) {
		return c.access, nil
	}
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {c.RefreshToken}}
	var out tokenResponse
	if err := postToken(ctx, c.http(), c.accounts(), c.ClientID, c.ClientSecret, form, &out); err != nil {
		return "", err
	}
	c.access = out.AccessToken
	c.exp = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	if out.RefreshToken != "" && out.RefreshToken != c.RefreshToken {
		c.RefreshToken = out.RefreshToken
		if c.OnRefreshToken != nil {
			c.OnRefreshToken(out.RefreshToken)
		}
	}
	return c.access, nil
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

func postToken(ctx context.Context, hc *http.Client, accounts, id, secret string, form url.Values, out *tokenResponse) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, accounts+"/api/token", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(id+":"+secret)))
	res, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("Spotify injoignable : %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		var e struct {
			Error       string `json:"error"`
			Description string `json:"error_description"`
		}
		_ = json.Unmarshal(body, &e)
		return &Error{Status: res.StatusCode, Reason: e.Error, Message: "Connexion à Spotify refusée (" + strings.TrimSpace(e.Error+" "+e.Description) + ") : relancer make spotify-login"}
	}
	if err := json.Unmarshal(body, out); err != nil || out.AccessToken == "" {
		return errors.New("réponse de Spotify illisible")
	}
	return nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	tok, err := c.token(ctx)
	if err != nil {
		return err
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = strings.NewReader(string(b))
	}
	req, err := http.NewRequestWithContext(ctx, method, c.api()+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http().Do(req)
	if err != nil {
		return fmt.Errorf("Spotify injoignable : %w", err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode >= 300 {
		return apiError(res.StatusCode, data)
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return errors.New("réponse de Spotify illisible")
		}
	}
	return nil
}

func apiError(status int, data []byte) error {
	var e struct {
		Error struct {
			Message string `json:"message"`
			Reason  string `json:"reason"`
		} `json:"error"`
	}
	_ = json.Unmarshal(data, &e)
	msg := "Spotify a refusé la demande"
	switch {
	case e.Error.Reason == "PREMIUM_REQUIRED":
		msg = "Spotify Premium est nécessaire pour lancer la lecture"
	case e.Error.Reason == "NO_ACTIVE_DEVICE" || status == http.StatusNotFound:
		msg = "Aucune enceinte Spotify active"
	case status == http.StatusUnauthorized:
		msg = "Session Spotify expirée : relancer make spotify-login"
	case status == http.StatusForbidden:
		msg = "Spotify refuse l'accès (compte non autorisé dans l'appli développeur ?)"
	case status == http.StatusTooManyRequests:
		msg = "Trop de demandes à Spotify, réessayer dans un instant"
	}
	if e.Error.Message != "" {
		msg += " (" + e.Error.Message + ")"
	}
	return &Error{Status: status, Reason: e.Error.Reason, Message: msg}
}

// Search returns up to limit tracks for a free-text query.
func (c *Client) Search(ctx context.Context, q string, limit int) ([]model.Track, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []model.Track{}, nil
	}
	if limit <= 0 || limit > 10 {
		limit = 10
	}
	v := url.Values{"q": {q}, "type": {"track"}, "limit": {fmt.Sprint(limit)}}
	if c.Market != "" {
		v.Set("market", c.Market)
	}
	var out struct {
		Tracks struct {
			Items []struct {
				URI        string `json:"uri"`
				Name       string `json:"name"`
				DurationMs int    `json:"duration_ms"`
				Artists    []struct {
					Name string `json:"name"`
				} `json:"artists"`
				Album struct {
					Name   string `json:"name"`
					Images []struct {
						URL   string `json:"url"`
						Width int    `json:"width"`
					} `json:"images"`
				} `json:"album"`
			} `json:"items"`
		} `json:"tracks"`
	}
	if err := c.do(ctx, http.MethodGet, "/search?"+v.Encode(), nil, &out); err != nil {
		return nil, err
	}
	tracks := []model.Track{}
	for _, it := range out.Tracks.Items {
		if !strings.HasPrefix(it.URI, "spotify:track:") {
			continue
		}
		names := make([]string, 0, len(it.Artists))
		for _, a := range it.Artists {
			names = append(names, a.Name)
		}
		t := model.Track{URI: it.URI, Title: it.Name, Artists: strings.Join(names, ", "), Album: it.Album.Name, Duration: it.DurationMs}
		// smallest cover that is still sharp at 48 px on a retina screen
		best := 0
		for _, im := range it.Album.Images {
			if strings.HasPrefix(im.URL, "https://i.scdn.co/") && (t.Cover == "" || (im.Width >= 96 && (best < 96 || im.Width < best))) {
				t.Cover, best = im.URL, im.Width
			}
		}
		tracks = append(tracks, t)
	}
	return tracks, nil
}

// Device is a Spotify Connect device.
type Device struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Active bool   `json:"is_active"`
}

func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	var out struct {
		Devices []Device `json:"devices"`
	}
	if err := c.do(ctx, http.MethodGet, "/me/player/devices", nil, &out); err != nil {
		return nil, err
	}
	return out.Devices, nil
}

var unaccent = strings.NewReplacer("à", "a", "â", "a", "ä", "a", "é", "e", "è", "e", "ê", "e", "ë", "e", "î", "i", "ï", "i", "ô", "o", "ö", "o", "ù", "u", "û", "u", "ü", "u", "ç", "c", "œ", "oe", "æ", "ae")

// Fold compares speaker names without case, accents or spacing (« Salon », « salon »).
func Fold(s string) string {
	s = unaccent.Replace(strings.ToLower(strings.TrimSpace(s)))
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), " ")
}

// Play starts a track (spotify:track:…) on the speaker named deviceName.
func (c *Client) Play(ctx context.Context, deviceName, uri string) error {
	if !strings.HasPrefix(uri, "spotify:track:") || strings.ContainsAny(uri, " /?&#") {
		return &Error{Status: http.StatusBadRequest, Message: "Titre Spotify invalide"}
	}
	devs, err := c.Devices(ctx)
	if err != nil {
		return err
	}
	want := Fold(deviceName)
	id := ""
	for _, d := range devs {
		if Fold(d.Name) == want {
			id = d.ID
			break
		}
	}
	if id == "" {
		names := make([]string, 0, len(devs))
		for _, d := range devs {
			names = append(names, d.Name)
		}
		seen := "aucune"
		if len(names) > 0 {
			seen = strings.Join(names, ", ")
		}
		return &Error{Status: http.StatusConflict, Reason: "DEVICE_NOT_FOUND", Message: fmt.Sprintf("L'enceinte « %s » n'est pas visible de Spotify (vues : %s). Lancer une fois Spotify sur l'enceinte depuis un téléphone de la maison.", deviceName, seen)}
	}
	return c.do(ctx, http.MethodPut, "/me/player/play?device_id="+url.QueryEscape(id), map[string]any{"uris": []string{uri}}, nil)
}

// AuthorizeURL is the page where the account owner allows Control.
func AuthorizeURL(clientID, state string) string {
	v := url.Values{"response_type": {"code"}, "client_id": {clientID}, "scope": {Scopes}, "redirect_uri": {RedirectURI}, "state": {state}}
	return DefaultAccounts + "/authorize?" + v.Encode()
}

// Exchange trades the authorization code for the refresh token.
func Exchange(ctx context.Context, hc *http.Client, accounts, clientID, secret, code string) (string, error) {
	if accounts == "" {
		accounts = DefaultAccounts
	}
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	var out tokenResponse
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {RedirectURI}}
	if err := postToken(ctx, hc, accounts, clientID, secret, form, &out); err != nil {
		return "", err
	}
	if out.RefreshToken == "" {
		return "", errors.New("Spotify n'a pas donné de jeton de renouvellement")
	}
	return out.RefreshToken, nil
}
