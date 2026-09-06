package egais

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// UtmProvider — клиент УТМ ЕГАИС.
type UtmProvider interface {
	Code() string
	// Check опрашивает самодиагностику УТМ.
	Check(ctx context.Context, utmURL string) (string, error)
	// SendDoc отправляет XML-документ во входящие УТМ.
	SendDoc(ctx context.Context, utmURL, xmlDoc string) (string, error)
}

var httpClient = &http.Client{Timeout: 20 * time.Second}

// UTM — универсальный транспортный модуль:
// GET {url}/diagnosis, POST {url}/xml (application/xml).
type UTM struct{}

func (UTM) Code() string { return "EGAIS_UTM" }

func trimURL(u string) string { return strings.TrimRight(u, "/") }

func (UTM) Check(ctx context.Context, utmURL string) (string, error) {
	if utmURL == "" {
		return "", fmt.Errorf("utm: utm_url required")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", trimURL(utmURL)+"/diagnosis", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("utm: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("utm: status %s", resp.Status)
	}
	b, _ := io.ReadAll(resp.Body)
	// diagnosis отдает JSON {"version": "..."} либо XML — разбираем оба.
	var diag struct {
		Version string `json:"version"`
		CN      string `json:"CN"`
	}
	if err := json.Unmarshal(b, &diag); err == nil && diag.Version != "" {
		return diag.Version, nil
	}
	// XML-вариант: ищем <version>.
	type diagXML struct {
		Version string `xml:"version"`
	}
	var dx diagXML
	if err := xml.Unmarshal(b, &dx); err == nil && dx.Version != "" {
		return dx.Version, nil
	}
	return strings.TrimSpace(string(b)), nil
}

func (UTM) SendDoc(ctx context.Context, utmURL, xmlDoc string) (string, error) {
	if utmURL == "" {
		return "", fmt.Errorf("utm: utm_url required")
	}
	if !strings.Contains(xmlDoc, "<") {
		return "", fmt.Errorf("utm: not an xml document")
	}
	req, err := http.NewRequestWithContext(ctx, "POST", trimURL(utmURL)+"/xml",
		strings.NewReader(xmlDoc))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/xml")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("utm: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("utm: status %s: %s", resp.Status, truncate(string(b), 200))
	}
	// Ответ УТМ: URL квитанции (/opt/out/...) либо XML.
	loc := resp.Header.Get("Location")
	if loc == "" {
		loc = strings.TrimSpace(string(b))
	}
	if loc == "" {
		loc = "accepted"
	}
	return loc, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
