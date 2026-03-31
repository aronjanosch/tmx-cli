package cmd

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/aronjanosch/tmx-cli/internal/config"
	"golang.org/x/term"
)

const (
	ciamLoginURL  = "https://ciam.prod.cookidoo.vorwerk-digital.com/login-srv/login"
	oauthStartURL = cookidooBase + "/oauth2/start?market=de&ui_locales=de-DE&rd=/planning/de-DE/my-week"
	browserUA     = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

type LoginCmd struct {
	Email    string `short:"e" help:"Email address."`
	Password string `short:"p" help:"Password (prompted if omitted)."`
}

func (l *LoginCmd) Run(ctx *Context) error {
	email := l.Email
	if email == "" {
		fmt.Print("Email: ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		email = strings.TrimSpace(scanner.Text())
	}

	password := l.Password
	if password == "" {
		fmt.Print("Password: ")
		b, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			return fmt.Errorf("reading password: %w", err)
		}
		password = string(b)
	}

	fmt.Println("Logging in...")
	cookies, err := login(email, password)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	if err := config.SaveCookies(cookies); err != nil {
		return fmt.Errorf("saving cookies: %w", err)
	}

	fmt.Printf("Logged in. %d cookies saved.\n", len(cookies))
	return nil
}

func login(email, password string) ([]*http.Cookie, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	cl := &http.Client{
		Jar:     jar,
		Timeout: 30 * time.Second,
	}

	baseHeaders := map[string]string{
		"User-Agent":      browserUA,
		"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"Accept-Language": "de-DE,de;q=0.9,en;q=0.8",
	}

	// Step 1: Start OAuth flow — follow redirects to Cidaas login page
	loginHTML, loginURL, err := get(cl, oauthStartURL, baseHeaders)
	if err != nil {
		return nil, fmt.Errorf("oauth start: %w", err)
	}

	// Extract requestId from login page HTML or final URL
	requestID := extractRequestID(loginHTML, loginURL)
	if requestID == "" {
		return nil, fmt.Errorf("could not find requestId in login page")
	}

	// Step 2: POST credentials to Cidaas
	form := url.Values{
		"requestId": {requestID},
		"username":  {email},
		"password":  {password},
	}
	postHeaders := map[string]string{
		"User-Agent":      browserUA,
		"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"Accept-Language": "de-DE,de;q=0.9,en;q=0.8",
		"Content-Type":    "application/x-www-form-urlencoded",
		"Origin":          "https://eu.login.vorwerk.com",
		"Referer":         loginURL,
	}

	resultHTML, finalURL, err := post(cl, ciamLoginURL, strings.NewReader(form.Encode()), postHeaders)
	if err != nil {
		return nil, fmt.Errorf("credentials POST: %w", err)
	}

	// Step 3: Follow JS/meta redirects if needed (up to 10 hops)
	cookidooURL, _ := url.Parse(cookidooBase)
	for i := 0; i < 10; i++ {
		if strings.Contains(finalURL, "cookidoo.de") && !strings.Contains(finalURL, "oauth2/start") {
			// Access the Cookidoo page to complete the OAuth callback
			html, u, err := get(cl, finalURL, baseHeaders)
			if err == nil {
				resultHTML = html
				finalURL = u
				if strings.Contains(u, "my-week") || strings.Contains(html, "is-authenticated") {
					break
				}
			}
		}

		// Check for JS or meta redirect in page
		next := extractRedirect(resultHTML, finalURL)
		if next == "" {
			break
		}
		resultHTML, finalURL, err = get(cl, next, baseHeaders)
		if err != nil {
			break
		}
	}

	// Step 4: Verify we got real auth cookies
	cookies := jar.Cookies(cookidooURL)
	for _, c := range cookies {
		if c.Name == "_oauth2_proxy" || c.Name == "v-authenticated" {
			return jar.Cookies(cookidooURL), nil
		}
	}

	// Detect wrong password / user not found
	lower := strings.ToLower(resultHTML)
	if strings.Contains(lower, "falsches passwort") || strings.Contains(lower, "incorrect") {
		return nil, fmt.Errorf("wrong password")
	}
	if strings.Contains(lower, "nicht gefunden") || strings.Contains(lower, "not found") {
		return nil, fmt.Errorf("email not found")
	}

	return nil, fmt.Errorf("no auth cookies received — check credentials")
}

func get(cl *http.Client, rawURL string, headers map[string]string) (body, finalURL string, err error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return "", "", err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := cl.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return string(raw), resp.Request.URL.String(), nil
}

func post(cl *http.Client, rawURL string, body io.Reader, headers map[string]string) (respBody, finalURL string, err error) {
	req, err := http.NewRequest("POST", rawURL, body)
	if err != nil {
		return "", "", err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := cl.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return string(raw), resp.Request.URL.String(), nil
}

var (
	requestIDFormRe = regexp.MustCompile(`name="requestId"\s+value="([^"]+)"`)
	requestIDURLRe  = regexp.MustCompile(`requestId=([^&"]+)`)
	jsRedirectRe    = regexp.MustCompile(`location\.href\s*=\s*["']([^"']+)["']`)
	metaRefreshRe   = regexp.MustCompile(`(?i)<meta[^>]+http-equiv="refresh"[^>]+url=([^"'\s>]+)`)
)

func extractRequestID(body, pageURL string) string {
	if m := requestIDFormRe.FindStringSubmatch(body); len(m) > 1 {
		return m[1]
	}
	if m := requestIDURLRe.FindStringSubmatch(pageURL); len(m) > 1 {
		return m[1]
	}
	return ""
}

func extractRedirect(body, currentURL string) string {
	if m := jsRedirectRe.FindStringSubmatch(body); len(m) > 1 {
		return resolveURL(currentURL, m[1])
	}
	if m := metaRefreshRe.FindStringSubmatch(body); len(m) > 1 {
		return resolveURL(currentURL, m[1])
	}
	return ""
}

func resolveURL(base, ref string) string {
	if strings.HasPrefix(ref, "http") {
		return ref
	}
	b, err := url.Parse(base)
	if err != nil {
		return ref
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return b.ResolveReference(r).String()
}
