package cmd

import (
	"encoding/json"
	"fmt"
)

type WhoamiCmd struct{}

type profileResponse struct {
	ID       string `json:"id"`
	UserInfo struct {
		Username    string `json:"username"`
		Description string `json:"description"`
	} `json:"userInfo"`
}

type subscription struct {
	Active             bool   `json:"active"`
	Type               string `json:"type"`
	ExtendedType       string `json:"extendedType"`
	Status             string `json:"status"`
	Expires            string `json:"expires"`
	StartDate          string `json:"startDate"`
	SubscriptionLevel  string `json:"subscriptionLevel"`
	CountryOfResidence string `json:"countryOfResidence"`
}

func (w *WhoamiCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}

	raw, err := cl.Get("/community/profile")
	if err != nil {
		return fmt.Errorf("fetching profile (are you logged in?): %w", err)
	}
	var profile profileResponse
	if err := json.Unmarshal(raw, &profile); err != nil {
		return fmt.Errorf("parsing profile: %w", err)
	}

	raw, err = cl.Get("/ownership/subscriptions")
	if err != nil {
		return fmt.Errorf("fetching subscriptions: %w", err)
	}
	var subs []subscription
	if err := json.Unmarshal(raw, &subs); err != nil {
		return fmt.Errorf("parsing subscriptions: %w", err)
	}

	// Pick the active subscription, or the most recent one as fallback.
	var current *subscription
	for i := range subs {
		if subs[i].Active {
			current = &subs[i]
			break
		}
	}
	if current == nil && len(subs) > 0 {
		current = &subs[0]
	}

	if ctx.JSON {
		out := map[string]any{
			"id":       profile.ID,
			"username": profile.UserInfo.Username,
		}
		if current != nil {
			out["subscription"] = current
		}
		return ctx.PrintJSON(out)
	}

	fmt.Printf("User:      %s (%s)\n", profile.UserInfo.Username, profile.ID)
	if current != nil {
		fmt.Printf("Plan:      %s (%s)\n", current.Type, current.Status)
		if current.Expires != "" {
			fmt.Printf("Expires:   %s\n", current.Expires)
		}
	} else {
		fmt.Println("Plan:      none")
	}
	return nil
}
