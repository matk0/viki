package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxDevelopmentTargetResponseBytes = 64 * 1024

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
		client: &http.Client{
			Timeout: 2 * time.Minute,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (target *httpDevelopmentTarget) Apply(ctx context.Context, implementation string) (string, error) {
	if target.url == "" {
		return "", fmt.Errorf("development target is not configured")
	}
	if target.token == "" {
		return "", fmt.Errorf("development target token is not configured")
	}
	payload := `{"implementation":` + strconv.Quote(implementation) + `}`
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.url+"/implement", strings.NewReader(payload))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+target.token)
	response, err := target.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("development target: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("development target returned %s", response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxDevelopmentTargetResponseBytes+1))
	if err != nil {
		return "", fmt.Errorf("read development receipt: %w", err)
	}
	if len(body) > maxDevelopmentTargetResponseBytes {
		return "", fmt.Errorf("development target response exceeds %d bytes", maxDevelopmentTargetResponseBytes)
	}
	var result struct {
		Receipt string `json:"receipt"`
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&result); err != nil {
		return "", fmt.Errorf("decode development receipt: %w", err)
	}
	result.Receipt = strings.TrimSpace(result.Receipt)
	if result.Receipt == "" {
		return "", fmt.Errorf("development target returned an empty receipt")
	}
	return result.Receipt, nil
}
