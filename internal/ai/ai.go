// Package ai generates text through a personal OpenAI-compatible endpoint
// and also provides images, audio transcription, OCR and reranking.
package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// Defaults used when nothing is configured.
const (
	DefaultModel      = "gpt-5.4-mini"
	DefaultTranscribe = "gpt-4o-mini-transcribe"
	DefaultTTS        = "gpt-4o-mini-tts"
)

// Client holds the endpoint and key. The service keys let one endpoint
// require a different key per feature (text, images, transcription, OCR,
// reranking); empty = the main key.
type Client struct {
	BaseURL         string
	Key             string
	Model           string
	TranscribeModel string
	// Per-service keys and models: one endpoint may require a key and a
	// model id per feature. Empty = the main key / model.
	ImageKey      string
	ImageModel    string
	TranscribeKey string
	OCRKey        string
	OCRModel      string
	RerankKey     string
	RerankModel   string
	TTSKey        string
	TTSModel      string
	HTTP          *http.Client
}

// New creates a client.
func New(baseURL, key, model, transcribe string) *Client {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if model == "" {
		model = DefaultModel
	}
	if transcribe == "" {
		transcribe = DefaultTranscribe
	}
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Key: key, Model: model, TranscribeModel: transcribe, HTTP: &http.Client{Timeout: 150 * time.Second}}
}

// Configured reports whether some key is set (main or per-service).
func (c *Client) Configured() bool {
	return c != nil && (c.Key != "" || c.ImageKey != "" || c.TranscribeKey != "" || c.OCRKey != "" || c.RerankKey != "")
}

// Usage is what one call cost in tokens.
type Usage struct {
	Model            string `json:"model"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// reasoning reports whether the model takes a reasoning_effort (gpt-5*, o-series).
func reasoning(model string) bool {
	m := strings.ToLower(model)
	if strings.Contains(m, "chat-latest") {
		return false
	}
	return strings.HasPrefix(m, "gpt-5") || (len(m) > 1 && m[0] == 'o' && m[1] >= '1' && m[1] <= '9')
}

// Complete sends a system + user prompt and returns the assistant text.
// With jsonMode the model is asked for a single JSON object.
func (c *Client) Complete(ctx context.Context, system, user string, jsonMode bool) (string, Usage, error) {
	u := Usage{Model: c.Model}
	if !c.Configured() {
		return "", u, errors.New("AI is not configured")
	}
	body := map[string]any{
		"model":    c.Model,
		"messages": []message{{Role: "system", Content: system}, {Role: "user", Content: user}},
	}
	if jsonMode {
		body["response_format"] = map[string]string{"type": "json_object"}
	}
	if reasoning(c.Model) {
		body["reasoning_effort"] = "low"
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return "", u, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", u, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode != http.StatusOK {
		return "", u, apiError(res.StatusCode, raw)
	}
	var out struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			Prompt     int `json:"prompt_tokens"`
			Completion int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", u, err
	}
	u.PromptTokens, u.CompletionTokens = out.Usage.Prompt, out.Usage.Completion
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", u, errors.New("AI returned no answer")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), u, nil
}

// JSON runs Complete in JSON mode and decodes into v.
func (c *Client) JSON(ctx context.Context, system, user string, v any) (Usage, error) {
	txt, u, err := c.Complete(ctx, system, user, true)
	if err != nil {
		return u, err
	}
	txt = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(txt), "```json"), "```")
	txt = strings.TrimSuffix(strings.TrimSpace(txt), "```")
	if err := json.Unmarshal([]byte(strings.TrimSpace(txt)), v); err != nil {
		return u, fmt.Errorf("AI answer was not valid JSON: %w", err)
	}
	return u, nil
}

// Transcribe turns recorded audio into text. lang is an ISO-639-1 hint or "".
func (c *Client) Transcribe(ctx context.Context, audio []byte, filename, lang string) (string, error) {
	if !c.Configured() {
		return "", errors.New("AI is not configured")
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("model", c.TranscribeModel)
	if lang != "" {
		_ = mw.WriteField("language", lang)
	}
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(audio); err != nil {
		return "", err
	}
	_ = mw.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/audio/transcriptions", &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.serviceKey(c.TranscribeKey))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		return "", apiError(res.StatusCode, raw)
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.Text), nil
}

// TTS turns text into speech (OpenAI-compatible /audio/speech). It returns
// the audio bytes. rate is 0.5..2.0 (1 = normal).
func (c *Client) TTS(ctx context.Context, text, model, voice string, rate float64) ([]byte, error) {
	key := c.serviceKey(c.TTSKey)
	if key == "" {
		return nil, errors.New("AI is not configured")
	}
	if model == "" {
		model = c.TTSModel
	}
	if model == "" {
		model = DefaultTTS
	}
	if rate <= 0 {
		rate = 1
	}
	body := map[string]any{"model": model, "input": text, "voice": voice}
	if voice == "" {
		delete(body, "voice")
	}
	body["rate"] = rate
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/audio/speech", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 20<<20))
	if res.StatusCode != http.StatusOK {
		return nil, apiError(res.StatusCode, raw)
	}
	if len(raw) == 0 {
		return nil, errors.New("the TTS model returned no audio")
	}
	return raw, nil
}

// Models lists available model ids (connection test).
func (c *Client) Models(ctx context.Context) ([]string, error) {
	if !c.Configured() {
		return nil, errors.New("AI is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode != http.StatusOK {
		return nil, apiError(res.StatusCode, raw)
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(out.Data))
	for _, d := range out.Data {
		ids = append(ids, d.ID)
	}
	return ids, nil
}

// Image generates one image with the configured model's image endpoint
// (OpenAI-compatible /images/generations). It returns the image bytes; the
// answer may carry a URL or base64 data.
func (c *Client) Image(ctx context.Context, prompt, size string) ([]byte, error) {
	if !c.Configured() {
		return nil, errors.New("AI is not configured")
	}
	body := map[string]any{
		"model":  c.ImageModel,
		"prompt": prompt,
		"n":      1,
	}
	// response_format is requested as b64_json only when the endpoint is
	// plain OpenAI: some compatible endpoints (litellm with Qwen-Image and
	// similar) reject the parameter, while already answering with b64_json.
	if strings.Contains(c.BaseURL, "api.openai.com") {
		body["response_format"] = "b64_json"
	}
	if c.ImageModel == "" {
		body["model"] = c.Model
	}
	if size != "" {
		body["size"] = size
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/images/generations", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.serviceKey(c.ImageKey))
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 20<<20))
	if res.StatusCode != http.StatusOK {
		return nil, apiError(res.StatusCode, raw)
	}
	var out struct {
		Data []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if len(out.Data) == 0 {
		return nil, errors.New("the image model returned no image")
	}
	d := out.Data[0]
	switch {
	case d.B64JSON != "":
		img, err := base64.StdEncoding.DecodeString(d.B64JSON)
		if err != nil {
			return nil, fmt.Errorf("image b64: %w", err)
		}
		return img, nil
	case d.URL != "":
		return c.fetchImage(ctx, d.URL)
	}
	return nil, errors.New("the image model returned no image data")
}

// fetchImage downloads a generated image URL (same key when the endpoint is self-hosted).
func (c *Client) fetchImage(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if strings.Contains(url, c.BaseURL) {
		req.Header.Set("Authorization", "Bearer "+c.Key)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("image HTTP %d", res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, 20<<20))
}

// serviceKey is the key for one feature: the feature's own key when set,
// else the main key.
func (c *Client) serviceKey(own string) string {
	if own != "" {
		return own
	}
	return c.Key
}

// OCR reads the text out of an image through a chat model that accepts
// image_url content (DeepSeek-OCR and similar). prompt is the instruction.
func (c *Client) OCR(ctx context.Context, img []byte, prompt string) (string, error) {
	key := c.serviceKey(c.OCRKey)
	if key == "" {
		return "", errors.New("AI is not configured")
	}
	b64 := "data:image/png;base64," + base64.StdEncoding.EncodeToString(img)
	body := map[string]any{
		"model":       c.OCRModel,
		"temperature": 0,
		"messages": []map[string]any{
			{"role": "user", "content": []map[string]any{
				{"type": "image_url", "image_url": map[string]string{"url": b64}},
				{"type": "text", "text": prompt},
			}},
		},
	}
	if c.OCRModel == "" {
		body["model"] = c.Model
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode != http.StatusOK {
		return "", apiError(res.StatusCode, raw)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", errors.New("the OCR model returned no answer")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

// RerankScore is one document's relevance for the query.
type RerankScore struct {
	Index int     `json:"index"`
	Score float64 `json:"score"`
}

// Rerank reorders documents by relevance to the query (Jina/Cohere-style
// /rerank endpoint). It degrades to the input order when the endpoint fails.
func (c *Client) Rerank(ctx context.Context, query string, docs []string, topN int) ([]RerankScore, error) {
	key := c.serviceKey(c.RerankKey)
	if key == "" {
		return nil, errors.New("AI is not configured")
	}
	if topN <= 0 || topN > len(docs) {
		topN = len(docs)
	}
	body := map[string]any{"model": c.RerankModel, "query": query, "documents": docs, "top_n": topN}
	if c.RerankModel == "" {
		body["model"] = c.Model
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/rerank", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode != http.StatusOK {
		return nil, apiError(res.StatusCode, raw)
	}
	var out struct {
		Results []struct {
			Index int     `json:"index"`
			Score float64 `json:"relevance_score"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	out2 := make([]RerankScore, 0, len(out.Results))
	for _, r := range out.Results {
		out2 = append(out2, RerankScore{Index: r.Index, Score: r.Score})
	}
	return out2, nil
}

func apiError(status int, raw []byte) error {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &e)
	if status == http.StatusUnauthorized {
		return errors.New("the API key was rejected (401)")
	}
	if e.Error.Message != "" {
		return fmt.Errorf("AI HTTP %d: %s", status, e.Error.Message)
	}
	return fmt.Errorf("AI HTTP %d", status)
}
