package coreupdate

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Release struct {
	Tag         string    `json:"tag"`
	Notes       string    `json:"notes"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"published_at"`

	assetURL string
	dgstURL  string
	digest   string
}

// XKeen downloads the same archives
func assetName(arch string) string {
	switch arch {
	case "arm64":
		return "Xray-linux-arm64-v8a.zip"
	case "mipsle":
		return "Xray-linux-mips32le.zip"
	case "mips":
		return "Xray-linux-mips32.zip"
	}
	return ""
}

func (u *Updater) fetchLatest(ctx context.Context) (Release, error) {
	var payload struct {
		TagName     string    `json:"tag_name"`
		Body        string    `json:"body"`
		HTMLURL     string    `json:"html_url"`
		PublishedAt time.Time `json:"published_at"`
		Assets      []struct {
			Name   string `json:"name"`
			URL    string `json:"browser_download_url"`
			Digest string `json:"digest"`
		} `json:"assets"`
	}

	resp, err := u.get(ctx, fmt.Sprintf("%s/repos/%s/releases/latest", u.apiBase, repo), "application/vnd.github+json")
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&payload); err != nil {
		return Release{}, fmt.Errorf("ответ GitHub не разобран: %w", err)
	}

	release := Release{Tag: payload.TagName, Notes: payload.Body, URL: payload.HTMLURL, PublishedAt: payload.PublishedAt}
	asset := assetName(u.arch)
	for _, a := range payload.Assets {
		switch a.Name {
		case asset:
			release.assetURL = a.URL
			release.digest = strings.TrimPrefix(strings.ToLower(a.Digest), "sha256:")
		case asset + ".dgst":
			release.dgstURL = a.URL
		}
	}
	if release.assetURL == "" {
		return Release{}, fmt.Errorf("в релизе %s нет %s", release.Tag, asset)
	}
	return release, nil
}

var dgstSHA256Re = regexp.MustCompile(`(?m)^SHA2-256=\s*([0-9a-fA-F]{64})\s*$`)

func (u *Updater) expectedSum(ctx context.Context, release Release) (string, error) {
	if len(release.digest) == 64 {
		return release.digest, nil
	}
	if release.dgstURL == "" {
		return "", fmt.Errorf("у релиза %s нет контрольной суммы — установка отменена", release.Tag)
	}

	resp, err := u.get(ctx, release.dgstURL, "")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var body strings.Builder
	scanner := bufio.NewScanner(io.LimitReader(resp.Body, 16<<10))
	for scanner.Scan() {
		body.WriteString(scanner.Text() + "\n")
	}
	m := dgstSHA256Re.FindStringSubmatch(body.String())
	if m == nil {
		return "", fmt.Errorf("в %s.dgst нет суммы SHA2-256", assetName(u.arch))
	}
	return strings.ToLower(m[1]), nil
}

func (u *Updater) get(ctx context.Context, url, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "xkeen-panel")
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

var xrayVersionRe = regexp.MustCompile(`Xray (\d+\.\d+\.\d+)`)

func parseVersion(s string) ([3]int, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+ "); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var v [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		v[i] = n
	}
	return v, true
}

func newer(current, latest string) bool {
	c, ok1 := parseVersion(current)
	l, ok2 := parseVersion(latest)
	if !ok1 || !ok2 {
		return false
	}
	for i := range c {
		if c[i] != l[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func sameVersion(a, b string) bool {
	x, ok1 := parseVersion(a)
	y, ok2 := parseVersion(b)
	return ok1 && ok2 && x == y
}
