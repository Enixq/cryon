package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	anyllm "github.com/mozilla-ai/any-llm-go"
)

// The adapter's SDK does not expose WithMaxRetries. Own this one HTTP exchange
// so a transport failure is neither repeated nor replaced by a retry error.
// Director remains Cryon's provider-independent PlanSearch/Select boundary.
func (d *Director) completeOpenAI(ctx context.Context, system string, input []byte, format *anyllm.ResponseFormat, tokens int) (string, error) {
	body := map[string]any{"model": d.model, "stream": false, "response_format": format, "messages": []any{map[string]string{"role": "system", "content": system}, map[string]string{"role": "user", "content": string(input)}}}
	model := strings.ToLower(d.model)
	reasoning := strings.HasPrefix(model, "gpt-5") || strings.HasPrefix(model, "o1") || strings.HasPrefix(model, "o3") || strings.HasPrefix(model, "o4")
	if reasoning {
		body["max_completion_tokens"] = tokens
		body["reasoning_effort"] = "low"
	} else {
		body["max_tokens"] = tokens
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(d.endpoint, "/")+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("Некорректный адрес AI API")
	}
	req.Header.Set("Authorization", "Bearer "+d.key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		return "", d.requestError(ctx, err)
	}
	defer resp.Body.Close()
	raw, err = io.ReadAll(io.LimitReader(resp.Body, 256*1024+1))
	if err != nil {
		return "", d.requestError(ctx, err)
	}
	if len(raw) > 256*1024 {
		return "", fmt.Errorf("Ответ AI API превысил допустимый размер")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", d.requestError(ctx, nil)
	}
	var result completionResponse
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") || bytes.HasPrefix(bytes.TrimSpace(raw), []byte("data:")) {
		// A few compatible gateways send SSE even for stream:false. Read its bounded
		// final content and usage; reasoning deltas never become playlist JSON.
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if line == "[DONE]" {
				continue
			}
			var chunk completionResponse
			if json.Unmarshal([]byte(line), &chunk) != nil {
				return "", fmt.Errorf("AI API вернул повреждённый поток ответа")
			}
			if chunk.Usage.TotalTokens > 0 || chunk.Usage.PromptTokens+chunk.Usage.CompletionTokens > 0 {
				result.Usage = chunk.Usage
			}
			for _, choice := range chunk.Choices {
				if choice.Index != 0 {
					return "", fmt.Errorf("AI API вернул несколько вариантов ответа")
				}
				if len(result.Choices) == 0 {
					result.Choices = append(result.Choices, completionChoice{})
				}
				target := &result.Choices[0]
				target.Message.Content += choice.Delta.Content + choice.Message.Content
				if choice.FinishReason != "" {
					target.FinishReason = choice.FinishReason
				}
			}
		}
	} else if json.Unmarshal(raw, &result) != nil {
		return "", fmt.Errorf("AI API вернул HTTP 200, но вместо JSON получен несовместимый ответ")
	}
	usage := result.Usage.TotalTokens
	if usage == 0 {
		usage = result.Usage.PromptTokens + result.Usage.CompletionTokens
	}
	d.tokens += max(0, usage)
	if len(result.Choices) != 1 {
		return "", fmt.Errorf("AI API не вернул единственный ответ модели; музыкальный поиск не начат")
	}
	choice := result.Choices[0]
	if choice.FinishReason == "length" {
		return "", fmt.Errorf("Модель исчерпала лимит ответа до завершения JSON; выберите менее затратный режим рассуждения или другую модель")
	}
	if choice.FinishReason != "" && choice.FinishReason != "stop" {
		return "", fmt.Errorf("Модель остановилась без завершённого музыкального плана")
	}
	if strings.TrimSpace(choice.Message.Content) == "" {
		return "", fmt.Errorf("Модель вернула пустой ответ; музыкальный поиск не начат")
	}
	return choice.Message.Content, nil
}

type completionChoice struct {
	Index        int    `json:"index"`
	FinishReason string `json:"finish_reason"`
	Message      struct {
		Content string `json:"content"`
	} `json:"message"`
	Delta struct {
		Content string `json:"content"`
	} `json:"delta"`
}
type completionResponse struct {
	Choices []completionChoice `json:"choices"`
	Usage   struct {
		TotalTokens      int `json:"total_tokens"`
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}
