package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

const base = "https://cookidoo.de"

// ErrUnauthorized signals an expired/invalid session. main maps it to exit code 3.
var ErrUnauthorized = errors.New("session expired — run: tmx login")

type Client struct {
	http    *http.Client
	jar     *cookiejar.Jar
	baseURL *url.URL

	// Relogin, if set, is called once on the first 401 to obtain fresh
	// session cookies; the failed request is then retried.
	Relogin      func() ([]*http.Cookie, error)
	reloginTried bool
}

func New(cookies []*http.Cookie) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	u, _ := url.Parse(base)

	if len(cookies) > 0 {
		jar.SetCookies(u, cookies)
	}

	return &Client{
		http: &http.Client{
			Jar:     jar,
			Timeout: 30 * time.Second,
		},
		jar:     jar,
		baseURL: u,
	}, nil
}

func (c *Client) Cookies() []*http.Cookie {
	return c.jar.Cookies(c.baseURL)
}

func (c *Client) Get(path string) ([]byte, error) {
	return c.do("GET", base+path, nil, nil)
}

func (c *Client) PostJSON(path string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return c.do("POST", base+path, body, map[string]string{
		"Content-Type": "application/json",
	})
}

func (c *Client) PutJSON(path string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return c.do("PUT", base+path, body, map[string]string{
		"Content-Type": "application/json",
	})
}

func (c *Client) Delete(path string) error {
	_, err := c.do("DELETE", base+path, nil, nil)
	return err
}

func (c *Client) PostForm(path string, values url.Values) ([]byte, error) {
	return c.do("POST", base+path, []byte(values.Encode()), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	})
}

func (c *Client) GetURL(rawURL string) ([]byte, error) {
	return c.do("GET", rawURL, nil, nil)
}

// Request performs a request with optional JSON payload and extra headers
// (e.g. vendor Accept headers required by the organize API).
func (c *Client) Request(method, path string, payload any, headers map[string]string) ([]byte, error) {
	var body []byte
	h := map[string]string{}
	for k, v := range headers {
		h[k] = v
	}
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = raw
		h["Content-Type"] = "application/json"
	}
	return c.do(method, base+path, body, h)
}

func (c *Client) do(method, rawURL string, body []byte, headers map[string]string) ([]byte, error) {
	raw, status, err := c.doOnce(method, rawURL, body, headers)
	// Retry idempotent GETs once on transport errors or server hiccups.
	if method == http.MethodGet && (err != nil || status >= 500) {
		time.Sleep(500 * time.Millisecond)
		raw, status, err = c.doOnce(method, rawURL, body, headers)
	}
	if err != nil {
		return nil, err
	}

	if status == http.StatusUnauthorized && c.Relogin != nil && !c.reloginTried {
		c.reloginTried = true
		cookies, loginErr := c.Relogin()
		if loginErr != nil {
			return nil, fmt.Errorf("%w (auto re-login failed: %v)", ErrUnauthorized, loginErr)
		}
		c.jar.SetCookies(c.baseURL, cookies)
		raw, status, err = c.doOnce(method, rawURL, body, headers)
		if err != nil {
			return nil, err
		}
	}

	if status == http.StatusUnauthorized {
		return nil, ErrUnauthorized
	}
	if status >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", status, truncate(strings.TrimSpace(string(raw)), 200))
	}

	return raw, nil
}

func (c *Client) doOnce(method, rawURL string, body []byte, headers map[string]string) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, rawURL, reader)
	if err != nil {
		return nil, 0, err
	}

	req.Header.Set("User-Agent", "tmx-cli/1.0")
	req.Header.Set("Accept", "application/json, text/html, */*")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}

	return raw, resp.StatusCode, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
