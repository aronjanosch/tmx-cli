package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/aronjanosch/tmx-cli/internal/auth"
	"github.com/aronjanosch/tmx-cli/internal/config"
	"golang.org/x/term"
)

type LoginCmd struct {
	Email         string `short:"e" env:"TMX_EMAIL" help:"Email address."`
	Password      string `short:"p" env:"TMX_PASSWORD" help:"Password (prompted if omitted)."`
	PasswordStdin bool   `name:"password-stdin" help:"Read password from stdin (for CI/agent use)."`
	Save          bool   `short:"s" help:"Store credentials (0600) for automatic re-login on session expiry."`
	Forget        bool   `help:"Delete stored credentials and exit."`
}

func (l *LoginCmd) Run(ctx *Context) error {
	if l.Forget {
		if err := config.ClearCredentials(); err != nil {
			return fmt.Errorf("deleting credentials: %w", err)
		}
		if ctx.JSON {
			return ctx.PrintJSON(map[string]string{"status": "credentials_deleted"})
		}
		fmt.Println("Stored credentials deleted.")
		return nil
	}

	email := l.Email
	if email == "" {
		fmt.Print("Email: ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		email = strings.TrimSpace(scanner.Text())
	}

	var password string
	switch {
	case l.PasswordStdin:
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		password = strings.TrimSpace(scanner.Text())
	case l.Password != "":
		password = l.Password
	default:
		fmt.Print("Password: ")
		b, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			return fmt.Errorf("reading password: %w", err)
		}
		password = string(b)
	}

	fmt.Println("Logging in...")
	cookies, err := auth.Login(email, password)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	if err := config.SaveCookies(cookies); err != nil {
		return fmt.Errorf("saving cookies: %w", err)
	}

	// Invalidate cached search token — new session needs fresh token.
	_ = config.ClearCache("search_token.json")

	if l.Save {
		if err := config.SaveCredentials(email, password); err != nil {
			return fmt.Errorf("saving credentials: %w", err)
		}
	}

	saved := ""
	if l.Save {
		saved = " Credentials stored for auto re-login."
	}
	fmt.Printf("Logged in. %d cookies saved.%s\n", len(cookies), saved)
	return nil
}
