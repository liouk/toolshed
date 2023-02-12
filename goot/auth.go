package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	tasks "google.golang.org/api/tasks/v1"
)

const configDir = ".config/toolshed/goot"

func configPath(filename string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, configDir, filename)
}

func authenticate(ctx context.Context) (*http.Client, error) {
	creds, err := os.ReadFile(configPath("credentials.json"))
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w\n\nPlace your OAuth credentials at %s", err, configPath("credentials.json"))
	}

	config, err := google.ConfigFromJSON(creds, tasks.TasksScope)
	if err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}

	token, err := loadToken()
	if err != nil {
		token, err = tokenFromWeb(ctx, config)
		if err != nil {
			return nil, err
		}
		if err := saveToken(token); err != nil {
			return nil, err
		}
	}

	return config.Client(ctx, token), nil
}

func loadToken() (*oauth2.Token, error) {
	f, err := os.Open(configPath("token.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var token oauth2.Token
	return &token, json.NewDecoder(f).Decode(&token)
}

func saveToken(token *oauth2.Token) error {
	path := configPath("token.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(token)
}

func tokenFromWeb(ctx context.Context, config *oauth2.Config) (*oauth2.Token, error) {
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	config.RedirectURL = "http://localhost:9876/callback"

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no code in callback")
			fmt.Fprintln(w, "Error: no authorization code received.")
			return
		}
		codeCh <- code
		fmt.Fprintln(w, "Authorization successful! You can close this tab.")
	})

	srv := &http.Server{Addr: ":9876", Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Opening browser for authorization...\n%s\n", authURL)
	openBrowser(authURL)

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return nil, fmt.Errorf("auth callback: %w", err)
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	srv.Shutdown(ctx)

	token, err := config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	return token, nil
}
