package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type config struct {
	HiddenListsByID []string `json:"hidden_lists_by_id"`
}

func loadConfig() config {
	f, err := os.Open(configPath("config.json"))
	if err != nil {
		return config{}
	}
	defer f.Close()

	var cfg config
	json.NewDecoder(f).Decode(&cfg)
	return cfg
}

func (c *config) addHiddenList(listID string) error {
	c.HiddenListsByID = append(c.HiddenListsByID, listID)
	return c.save()
}

func (c *config) save() error {
	path := configPath("config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(c)
}
