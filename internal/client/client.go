package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"
)

const base = "https://cookidoo.de"

type Client struct {
	http    *http.Client
	jar     *cookiejar.Jar
	baseURL *url.URL
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
	return c.do("POST", base+path, bytes.NewReader(body), map[string]string{
		"Content-Type": "application/json",
	})
}

func (c *Client) PutJSON(path string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return c.do("PUT", base+path, bytes.NewReader(body), map[string]string{
		"Content-Type": "application/json",
	})
}

func (c *Client) Delete(path string) error {
	_, err := c.do("DELETE", base+path, nil, nil)
	return err
}

func (c *Client) PostForm(path string, values url.Values) ([]byte, error) {
	return c.do("POST", base+path, bytes.NewBufferString(values.Encode()), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	})
}

func (c *Client) GetURL(rawURL string) ([]byte, error) {
	return c.do("GET", rawURL, nil, nil)
}

func (c *Client) do(method, rawURL string, body io.Reader, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "tmx-cli/1.0")
	req.Header.Set("Accept", "application/json, text/html, */*")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}

	return raw, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
