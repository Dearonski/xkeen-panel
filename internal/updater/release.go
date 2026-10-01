package updater

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"
)

const sumsAsset = "SHA256SUMS"

type Release struct {
	Tag         string    `json:"tag"`
	Notes       string    `json:"notes"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"published_at"`

	assetURL string
	sumsURL  string
}

// install.sh resolves the same names, so they must never change.
func AssetName(goos, goarch string) string {
	if goos != "linux" {
		return ""
	}
	switch goarch {
	case "arm64":
		return "xkeen-panel-aarch64"
	case "mipsle":
		return "xkeen-panel-mipsel"
	}
	return ""
}

func currentAsset() string {
	return AssetName(runtime.GOOS, runtime.GOARCH)
}

func (u *Updater) fetchLatest(ctx context.Context) (Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", u.apiBase, u.repo)

	var payload struct {
		TagName     string    `json:"tag_name"`
		Body        string    `json:"body"`
		HTMLURL     string    `json:"html_url"`
		PublishedAt time.Time `json:"published_at"`
		Assets      []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}

	resp, err := u.get(ctx, url, "application/vnd.github+json")
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return Release{}, fmt.Errorf("ответ GitHub не разобран: %w", err)
	}

	release := Release{
		Tag:         payload.TagName,
		Notes:       payload.Body,
		URL:         payload.HTMLURL,
		PublishedAt: payload.PublishedAt,
	}
	for _, asset := range payload.Assets {
		switch asset.Name {
		case u.asset:
			release.assetURL = asset.URL
		case sumsAsset:
			release.sumsURL = asset.URL
		}
	}

	return release, nil
}

func (u *Updater) expectedSum(ctx context.Context, release Release) (string, error) {
	if release.sumsURL == "" {
		return "", fmt.Errorf("в релизе %s нет %s — установка без проверки суммы отменена", release.Tag, sumsAsset)
	}

	resp, err := u.get(ctx, release.sumsURL, "")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(io.LimitReader(resp.Body, 64<<10))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == u.asset {
			return strings.ToLower(fields[0]), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("%s не прочитан: %w", sumsAsset, err)
	}

	return "", fmt.Errorf("в %s нет суммы для %s", sumsAsset, u.asset)
}

func (u *Updater) get(ctx context.Context, url, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "xkeen-panel/"+u.current)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("GitHub ограничил запросы (HTTP %d), попробуйте позже", resp.StatusCode)
		}
		return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}

	return resp, nil
}
