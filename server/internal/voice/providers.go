package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// ---------- speech → text ----------

// OpenAITranscriber calls an OpenAI-compatible /v1/audio/transcriptions
// (speaches / faster-whisper on the NAS by default).
type OpenAITranscriber struct {
	URL      string // base URL (http://host:8000) or the full …/audio/transcriptions URL
	Model    string // e.g. Systran/faster-whisper-small
	Language string // "fr"
	APIKey   string // optional (STT_API_KEY)
	HTTP     *http.Client
}

func (t *OpenAITranscriber) endpoint() string {
	u := strings.TrimRight(t.URL, "/")
	switch {
	case strings.HasSuffix(u, "/audio/transcriptions"):
		return u
	case strings.HasSuffix(u, "/v1"):
		return u + "/audio/transcriptions"
	default:
		return u + "/v1/audio/transcriptions"
	}
}

func (t *OpenAITranscriber) Transcribe(ctx context.Context, audio io.Reader, contentType string) (string, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	name := "audio" + extFor(contentType)
	fw, err := mw.CreateFormFile("file", name)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(fw, audio); err != nil {
		return "", err
	}
	_ = mw.WriteField("model", t.Model)
	if t.Language != "" {
		_ = mw.WriteField("language", t.Language)
	}
	_ = mw.WriteField("response_format", "json")
	_ = mw.WriteField("temperature", "0")
	if err := mw.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", t.endpoint(), &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if t.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.APIKey)
	}
	resp, err := client(t.HTTP).Do(req)
	if err != nil {
		return "", fmt.Errorf("transcription : %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("transcription : HTTP %d %s", resp.StatusCode, clip(b))
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", fmt.Errorf("transcription : réponse illisible : %w", err)
	}
	return strings.TrimSpace(out.Text), nil
}

func extFor(ct string) string {
	ct = strings.ToLower(ct)
	switch {
	case strings.Contains(ct, "webm"):
		return ".webm"
	case strings.Contains(ct, "ogg"):
		return ".ogg"
	case strings.Contains(ct, "mp4"), strings.Contains(ct, "m4a"), strings.Contains(ct, "aac"):
		return ".m4a" // Safari / iPhone
	case strings.Contains(ct, "mpeg"), strings.Contains(ct, "mp3"):
		return ".mp3"
	case strings.Contains(ct, "wav"):
		return ".wav"
	}
	return ".webm"
}

// ---------- text → draft : Anthropic ----------

// Anthropic uses the Messages API with a forced tool call, so the answer is
// always the JSON of the schema.
type Anthropic struct {
	APIKey string // ANTHROPIC_API_KEY
	Model  string // e.g. claude-haiku-5-5
	URL    string // default https://api.anthropic.com
	HTTP   *http.Client
}

func (a *Anthropic) Understand(ctx context.Context, text string, c Context) (Draft, error) {
	if a.APIKey == "" {
		return Draft{}, errors.New("ANTHROPIC_API_KEY manquante dans .env")
	}
	base := strings.TrimRight(a.URL, "/")
	if base == "" {
		base = "https://api.anthropic.com"
	}
	reqBody := map[string]any{
		"model":       a.Model,
		"max_tokens":  400,
		"temperature": 0,
		"system":      SystemPrompt(c),
		"tools": []map[string]any{{
			"name":         "evenement",
			"description":  "L'événement compris dans la phrase.",
			"input_schema": Schema,
		}},
		"tool_choice": map[string]any{"type": "tool", "name": "evenement"},
		"messages":    []map[string]any{{"role": "user", "content": text}},
	}
	b, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", base+"/v1/messages", bytes.NewReader(b))
	if err != nil {
		return Draft{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := client(a.HTTP).Do(req)
	if err != nil {
		return Draft{}, fmt.Errorf("Claude : %w", err)
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(rb, &e) == nil && e.Error.Message != "" {
			return Draft{}, fmt.Errorf("Claude : HTTP %d : %s", resp.StatusCode, e.Error.Message)
		}
		return Draft{}, fmt.Errorf("Claude : HTTP %d %s", resp.StatusCode, clip(rb))
	}
	var out struct {
		Content []struct {
			Type  string          `json:"type"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
			Text  string          `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(rb, &out); err != nil {
		return Draft{}, fmt.Errorf("Claude : réponse illisible : %w", err)
	}
	for _, c := range out.Content {
		if c.Type == "tool_use" && c.Name == "evenement" {
			var d Draft
			if err := json.Unmarshal(c.Input, &d); err != nil {
				return Draft{}, fmt.Errorf("Claude : événement illisible : %w", err)
			}
			return d, nil
		}
	}
	for _, c := range out.Content { // should not happen with tool_choice, but be lenient
		if c.Type == "text" {
			return decodeDraft(c.Text)
		}
	}
	return Draft{}, errors.New("Claude : pas d'événement dans la réponse")
}

// ---------- text → draft : OpenAI-compatible (local model) ----------

// OpenAIChat speaks /v1/chat/completions: Ollama (http://host:11434/v1),
// llama.cpp server, vLLM, LM Studio… — the way to a local model.
type OpenAIChat struct {
	URL    string // base URL ending in /v1
	Model  string // e.g. qwen2.5:3b
	APIKey string // LLM_API_KEY, optional
	HTTP   *http.Client
}

func (o *OpenAIChat) Understand(ctx context.Context, text string, c Context) (Draft, error) {
	schema, _ := json.Marshal(Schema)
	sys := SystemPrompt(c) + "\nRéponds uniquement par un objet JSON conforme à ce schéma, sans texte autour :\n" + string(schema)
	reqBody := map[string]any{
		"model":           o.Model,
		"temperature":     0,
		"max_tokens":      400,
		"response_format": map[string]any{"type": "json_object"},
		"messages": []map[string]any{
			{"role": "system", "content": sys},
			{"role": "user", "content": text},
		},
	}
	b, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(o.URL, "/")+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return Draft{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if o.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.APIKey)
	}
	resp, err := client(o.HTTP).Do(req)
	if err != nil {
		return Draft{}, fmt.Errorf("modèle : %w", err)
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		return Draft{}, fmt.Errorf("modèle : HTTP %d %s", resp.StatusCode, clip(rb))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rb, &out); err != nil || len(out.Choices) == 0 {
		return Draft{}, fmt.Errorf("modèle : réponse illisible %s", clip(rb))
	}
	return decodeDraft(out.Choices[0].Message.Content)
}

func client(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return &http.Client{Timeout: 60 * time.Second}
}

// ---------- text → draft : claude-gateway ----------

// Gateway calls claude-gateway, the homelab's Claude service (the owner's
// subscription, no API key): POST /v1/run with a profile (calendar: Haiku,
// little reasoning), the shared system prompt and the schema; the answer
// is the structured JSON of the schema. The gateway, not this app, chooses
// the model.
type Gateway struct {
	URL     string // http://claude.home.arpa
	Token   string // CLAUDE_GATEWAY_TOKEN: control's client token at the gateway
	Profile string // default calendar
	HTTP    *http.Client
}

func (g *Gateway) Understand(ctx context.Context, text string, c Context) (Draft, error) {
	if g.Token == "" {
		return Draft{}, errors.New("CLAUDE_GATEWAY_TOKEN manquant")
	}
	profile := g.Profile
	if profile == "" {
		profile = "calendar"
	}
	b, _ := json.Marshal(map[string]any{"profile": profile, "system": SystemPrompt(c), "input": text, "schema": Schema})
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(g.URL, "/")+"/v1/run", bytes.NewReader(b))
	if err != nil {
		return Draft{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.Token)
	resp, err := client(g.HTTP).Do(req)
	if err != nil {
		return Draft{}, fmt.Errorf("Claude (gateway) : %w", err)
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out struct {
		Result     string          `json:"result"`
		Structured json.RawMessage `json:"structured"`
		Error      string          `json:"error"`
	}
	_ = json.Unmarshal(rb, &out)
	if resp.StatusCode != 200 {
		if out.Error != "" {
			return Draft{}, fmt.Errorf("Claude (gateway) : HTTP %d : %s", resp.StatusCode, out.Error)
		}
		return Draft{}, fmt.Errorf("Claude (gateway) : HTTP %d %s", resp.StatusCode, clip(rb))
	}
	if len(out.Structured) > 0 && string(out.Structured) != "null" {
		var d Draft
		if err := json.Unmarshal(out.Structured, &d); err != nil {
			return Draft{}, fmt.Errorf("Claude (gateway) : événement illisible : %w", err)
		}
		return d, nil
	}
	return decodeDraft(out.Result)
}
