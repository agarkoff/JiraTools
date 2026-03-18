package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Config struct {
	URL   string // e.g. http://localhost:11434
	Model string // e.g. qwen3-coder:30b
}

func (c Config) IsConfigured() bool {
	return c.URL != "" && c.Model != ""
}

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
	Options struct {
		Temperature float64 `json:"temperature"`
	} `json:"options"`
}

type ollamaResponse struct {
	Response string `json:"response"`
}

func Generate(cfg Config, prompt string) (string, error) {
	reqBody := ollamaRequest{
		Model:  cfg.Model,
		Prompt: prompt,
		Stream: false,
	}
	reqBody.Options.Temperature = 0.1

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	url := strings.TrimRight(cfg.URL, "/") + "/api/generate"
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("ollama HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result ollamaResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	return result.Response, nil
}
