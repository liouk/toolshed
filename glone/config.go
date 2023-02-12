package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type orgEntry struct {
	Name          string
	CloneDir      string
	ForkCloneDirs map[string]string
	Exclude       map[string]bool
}

type config struct {
	Orgs   []orgEntry
	Editor string
}

type rawConfig struct {
	Orgs   []yaml.Node `yaml:"orgs"`
	Editor string      `yaml:"editor"`
}

type rawOrgObj struct {
	Name          string            `yaml:"name"`
	CloneDir      string            `yaml:"clone_dir"`
	ForkCloneDirs map[string]string `yaml:"fork_clone_dirs"`
	Exclude       []string          `yaml:"exclude"`
}

func loadConfig() (*config, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("could not determine config dir: %w", err)
	}

	path := dir + "/toolshed/glone/config.yaml"
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read %s: %w", path, err)
	}

	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("could not parse %s: %w", path, err)
	}

	if len(raw.Orgs) == 0 {
		return nil, fmt.Errorf("no orgs configured in %s", path)
	}

	cfg := config{Editor: raw.Editor}
	for _, node := range raw.Orgs {
		switch node.Kind {
		case yaml.MappingNode:
			var obj rawOrgObj
			if err := node.Decode(&obj); err != nil {
				return nil, fmt.Errorf("could not parse org entry: %w", err)
			}
			if obj.Name == "" {
				return nil, fmt.Errorf("org entry missing 'name' in %s", path)
			}
			if obj.CloneDir == "" {
				return nil, fmt.Errorf("org %q missing 'clone_dir' in %s", obj.Name, path)
			}
			cloneDir, err := expandTilde(obj.CloneDir)
			if err != nil {
				return nil, err
			}
			forkCloneDirs := make(map[string]string)
			for parentOrg, dir := range obj.ForkCloneDirs {
				expanded, err := expandTilde(dir)
				if err != nil {
					return nil, err
				}
				forkCloneDirs[parentOrg] = expanded
			}
			exclude := make(map[string]bool, len(obj.Exclude))
			for _, name := range obj.Exclude {
				exclude[name] = true
			}
			cfg.Orgs = append(cfg.Orgs, orgEntry{
				Name:          obj.Name,
				CloneDir:      cloneDir,
				ForkCloneDirs: forkCloneDirs,
				Exclude:       exclude,
			})
		default:
			return nil, fmt.Errorf("org entries must be objects with 'name' and 'clone_dir' in %s", path)
		}
	}

	return &cfg, nil
}

func expandTilde(path string) (string, error) {
	if len(path) == 0 || path[0] != '~' {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home dir: %w", err)
	}
	return home + path[1:], nil
}
