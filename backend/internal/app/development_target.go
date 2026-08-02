package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type developmentTarget interface {
	Apply(context.Context, string) (string, error)
}

type httpDevelopmentTarget struct {
	url    string
	token  string
	client *http.Client
}

func newHTTPDevelopmentTarget(url, token string) developmentTarget {
	return &httpDevelopmentTarget{
		url: strings.TrimRight(strings.TrimSpace(url), "/"), token: strings.TrimSpace(token),
		client: &http.Client{Timeout: 2 * time.Minute},
	}
}

func (target *httpDevelopmentTarget) Apply(ctx context.Context, implementation string) (string, error) {
	if target.url == "" {
		return "", fmt.Errorf("development target is not configured")
	}
	payload := `{"implementation":` + strconv.Quote(implementation) + `}`
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.url+"/implement", strings.NewReader(payload))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	if target.token != "" {
		request.Header.Set("Authorization", "Bearer "+target.token)
	}
	response, err := target.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("development target: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("development target returned %s", response.Status)
	}
	var result struct {
		Receipt string `json:"receipt"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode development receipt: %w", err)
	}
	result.Receipt = strings.TrimSpace(result.Receipt)
	if result.Receipt == "" {
		return "", fmt.Errorf("development target returned an empty receipt")
	}
	return result.Receipt, nil
}
