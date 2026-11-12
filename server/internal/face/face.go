// Package face talks to the homelab's face recognition service (dépôt
// ahomelab/face): a photo in, the id of an enrolled person out. The service
// only knows ids and face prints; who the person is (name, calendar) is in
// control.yaml.
//
// Photos only pass through memory: never written, never logged.
package face

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// MaxPhoto is the largest photo accepted (the service's own limit).
const MaxPhoto = 8 << 20

// Score is one person and how close their prints are to the photo (cosine, 0-1).
type Score struct {
	ID    string  `json:"id"`
	Score float64 `json:"score"`
}

// Result of an identification. Match is nil without a face or under the threshold.
type Result struct {
	Faces     int     `json:"faces"`
	Match     *Score  `json:"match"`
	Best      *Score  `json:"best"`
	Second    *Score  `json:"second"`
	Threshold float64 `json:"threshold"`
	MS        float64 `json:"ms"`
}

// Enrolled: one more print kept for a person.
type Enrolled struct {
	ID      string   `json:"id"`
	Samples int      `json:"samples"`
	Own     *float64 `json:"own"` // likeness with the person's previous photos (nil for the first one)
}

// Person as the service knows it: an id and how many prints.
type Person struct {
	ID      string `json:"id"`
	Samples int    `json:"samples"`
}

// Error is a refusal from the service. Status 409/422 carry a message meant for
// the person taking the photo (no face, two faces, already enrolled as someone else…).
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("service face : HTTP %d : %s", e.Status, e.Message)
}

// ForPerson says whether the refusal is about the photo itself (shown as is).
func (e *Error) ForPerson() bool { return e.Status == 409 || e.Status == 422 || e.Status == 400 }

// Recognizer is what the server needs (a fake in tests).
type Recognizer interface {
	Identify(ctx context.Context, photo []byte) (Result, error)
	Enroll(ctx context.Context, id string, photo []byte) (Enrolled, error)
	Forget(ctx context.Context, id string) error
	People(ctx context.Context) ([]Person, error)
}

// Client calls the service over HTTP with its bearer token.
type Client struct {
	URL   string // http://face.home.arpa
	Token string // FACE_API_KEY
	HTTP  *http.Client
}

func (c *Client) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (c *Client) do(ctx context.Context, method, path string, body []byte, out any) error {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.URL, "/")+path, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return fmt.Errorf("service face injoignable : %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		msg := strings.TrimSpace(string(b))
		if json.Unmarshal(b, &e) == nil && e.Error.Message != "" {
			msg = e.Error.Message
		}
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return &Error{Status: resp.StatusCode, Message: msg}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("service face : réponse illisible : %w", err)
	}
	return nil
}

func (c *Client) Identify(ctx context.Context, photo []byte) (Result, error) {
	var r Result
	err := c.do(ctx, "POST", "/v1/identify", photo, &r)
	return r, err
}

func (c *Client) Enroll(ctx context.Context, id string, photo []byte) (Enrolled, error) {
	var e Enrolled
	err := c.do(ctx, "POST", "/v1/people/"+url.PathEscape(id)+"/samples", photo, &e)
	return e, err
}

// Forget removes every print of the person; a person without any is not an error.
func (c *Client) Forget(ctx context.Context, id string) error {
	err := c.do(ctx, "DELETE", "/v1/people/"+url.PathEscape(id), nil, nil)
	var fe *Error
	if errors.As(err, &fe) && fe.Status == http.StatusNotFound {
		return nil
	}
	return err
}

func (c *Client) People(ctx context.Context) ([]Person, error) {
	var out struct {
		People []Person `json:"people"`
	}
	err := c.do(ctx, "GET", "/v1/people", nil, &out)
	return out.People, err
}

// Health: GET /health (no token), for `control check`.
func (c *Client) Health(ctx context.Context) (int, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(c.URL, "/")+"/health", nil)
	if err != nil {
		return 0, err
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}

// Sniff: only JPEG and PNG photos are passed on.
func Sniff(b []byte) bool {
	return bytes.HasPrefix(b, []byte{0xff, 0xd8, 0xff}) || bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n"))
}
