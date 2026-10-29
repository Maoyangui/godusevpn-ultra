package trial

// Package trial provides the small, server-backed free-trial client used by the
// desktop shell. The server owns the plan and the monthly limit; this package
// only keeps a random installation key and never sends machine identifiers.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const DefaultEndpoint = "https://godusevpn.org/app/api/trial"
const DefaultPurchaseURL = "https://godusevpn.org/subscription/"

var ErrInvalidEndpoint = errors.New("试用服务地址无效")

type Status struct {
	Status      string `json:"status"`
	Enabled     bool   `json:"enabled"`
	Plan        string `json:"plan"`
	Month       string `json:"month"`
	Claimed     bool   `json:"claimed"`
	PurchaseURL string `json:"purchaseUrl"`
	Message     string `json:"message"`
}

type ClaimResult struct {
	OK              bool   `json:"ok"`
	Status          string `json:"status"`
	Plan            string `json:"plan"`
	SubscriptionURL string `json:"subscriptionUrl"`
	PurchaseURL     string `json:"purchaseUrl"`
	ExpiresAt       int64  `json:"expiresAt"`
	Message         string `json:"message"`
}

type Client struct {
	Endpoint string
	DataDir  string
	HTTP     *http.Client
}

func New(endpoint, dataDir string) *Client {
	if strings.TrimSpace(endpoint) == "" {
		endpoint = DefaultEndpoint
	}
	c := &Client{Endpoint: strings.TrimRight(strings.TrimSpace(endpoint), "/"), DataDir: dataDir}
	c.HTTP = &http.Client{Timeout: 15 * time.Second}
	return c
}

func (c *Client) devicePath() string { return filepath.Join(c.DataDir, "trial-device.json") }

func (c *Client) DeviceID() (string, error) {
	if strings.TrimSpace(c.DataDir) == "" {
		return "", errors.New("试用设备存储目录为空")
	}
	// Prefer a local, one-way digest of the OS machine identity. The raw value
	// never leaves the client; this prevents deleting trial-device.json from
	// minting a new device on Windows/Linux while keeping a random fallback for
	// platforms that do not expose a stable OS identifier to this package.
	if stable := platformDeviceID(); stable != "" {
		sum := sha256.Sum256([]byte("godusevpn-ultra-trial-v1:" + stable))
		return base64.RawURLEncoding.EncodeToString(sum[:]), nil
	}
	p := c.devicePath()
	if b, err := os.ReadFile(p); err == nil {
		var v struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(b, &v) == nil && validDeviceID(v.ID) {
			return v.ID, nil
		}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("生成试用设备标识: %w", err)
	}
	id := base64.RawURLEncoding.EncodeToString(raw)
	if err := os.MkdirAll(c.DataDir, 0o700); err != nil {
		return "", fmt.Errorf("创建试用设备目录: %w", err)
	}
	b, _ := json.Marshal(struct {
		ID string `json:"id"`
	}{id})
	// Include the generated key in the temporary name so simultaneous status and
	// claim calls cannot overwrite one another's temporary file.
	tmp := p + ".tmp-" + id[:8]
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return "", fmt.Errorf("保存试用设备标识: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		// Another caller may have won the first-write race. Reuse its key so a
		// single install never creates two monthly identities.
		if existing, readErr := os.ReadFile(p); readErr == nil {
			var v struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(existing, &v) == nil && validDeviceID(v.ID) {
				return v.ID, nil
			}
		}
		return "", fmt.Errorf("保存试用设备标识: %w", err)
	}
	return id, nil
}

func validDeviceID(id string) bool {
	return len(id) >= 32 && len(id) <= 128 && !strings.ContainsAny(id, " \t\r\n")
}

func (c *Client) request(ctx context.Context, method, suffix string, body any, out any) error {
	if c == nil || !validEndpoint(c.Endpoint) {
		return ErrInvalidEndpoint
	}
	device, err := c.DeviceID()
	if err != nil {
		return err
	}
	var rd io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return e
		}
		rd = strings.NewReader(string(b))
	}
	u := strings.TrimRight(c.Endpoint, "/") + "/" + strings.TrimLeft(suffix, "/")
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-GodUseVPN-Device", device)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 512*1024))
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
			Status  string `json:"status"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Message == "" {
			e.Message = http.StatusText(res.StatusCode)
		}
		if e.Status != "" {
			return fmt.Errorf("试用服务[%s]: %s", e.Status, e.Message)
		}
		return fmt.Errorf("试用服务(%d): %s", res.StatusCode, e.Message)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("试用服务响应无效: %w", err)
		}
	}
	return nil
}

func (c *Client) Status(ctx context.Context) (Status, error) {
	var out Status
	err := c.request(ctx, http.MethodGet, "status", nil, &out)
	if out.PurchaseURL == "" {
		out.PurchaseURL = DefaultPurchaseURL
	}
	return out, err
}

func (c *Client) Claim(ctx context.Context) (ClaimResult, error) {
	id, err := c.DeviceID()
	if err != nil {
		return ClaimResult{}, err
	}
	var out ClaimResult
	err = c.request(ctx, http.MethodPost, "claim", map[string]string{"deviceId": id}, &out)
	if out.PurchaseURL == "" {
		out.PurchaseURL = DefaultPurchaseURL
	}
	return out, err
}

func validEndpoint(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}
