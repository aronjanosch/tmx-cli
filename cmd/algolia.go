package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aronjanosch/tmx-cli/internal/client"
	"github.com/aronjanosch/tmx-cli/internal/config"
)

const algoliaAppID = "3TA8NT85XJ"

var errAlgoliaAuth = errors.New("algolia token rejected")

type searchTokenCache struct {
	APIKey     string  `json:"apiKey"`
	ValidUntil float64 `json:"validUntil"`
}

func getSearchToken(cl *client.Client, force bool) (string, error) {
	if !force {
		var cached searchTokenCache
		if err := config.LoadCache("search_token.json", &cached); err == nil {
			if cached.ValidUntil > float64(time.Now().Unix()+300) {
				return cached.APIKey, nil
			}
		}
	}

	raw, err := cl.Get("/search/api/subscription/token")
	if err != nil {
		return "", err
	}

	var data searchTokenCache
	if err := json.Unmarshal(raw, &data); err != nil {
		return "", fmt.Errorf("parsing search token: %w", err)
	}

	_ = config.SaveCache("search_token.json", data)
	return data.APIKey, nil
}

func algoliaQuery(token, index string, params map[string]any, out any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://%s-dsn.algolia.net/1/indexes/%s/query", algoliaAppID, index)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("X-Algolia-Application-Id", algoliaAppID)
	req.Header.Set("X-Algolia-API-Key", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return errAlgoliaAuth
	}
	if resp.StatusCode >= 400 {
		var msg struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &msg)
		return fmt.Errorf("search failed (HTTP %d): %s", resp.StatusCode, msg.Message)
	}

	return json.Unmarshal(raw, out)
}

// algoliaQueryAuto runs an Algolia query with the cached search token,
// transparently fetching a fresh token once if the cached one was rejected.
func algoliaQueryAuto(cl *client.Client, index string, params map[string]any, out any) error {
	token, err := getSearchToken(cl, false)
	if err != nil {
		return fmt.Errorf("could not get search token (are you logged in?): %w", err)
	}
	err = algoliaQuery(token, index, params, out)
	if errors.Is(err, errAlgoliaAuth) {
		token, err = getSearchToken(cl, true)
		if err != nil {
			return fmt.Errorf("could not refresh search token: %w", err)
		}
		err = algoliaQuery(token, index, params, out)
	}
	return err
}
