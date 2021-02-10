package config

import (
	"fmt"
	"io/ioutil"

	yaml "gopkg.in/yaml.v3"
)

// Config contains configuration values for a disk-manager run
type Config struct {
	TargetAnnotation string `yaml:"targetAnnotation"`
	GoogleProject    string `yaml:"googleProject"`
	Zone             string `yaml:"zone"`
	Region           string `yaml:"region"`
}

// Read attempts to parse the file at configPath and create build a config struct from it
func Read(configPath string) (*Config, error) {
	configBytes, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("Error reading config file: %v", err)
	}
	config := new(Config)
	if err := yaml.Unmarshal(configBytes, config); err != nil {
		return nil, fmt.Errorf("Error parsing config: %v", err)
	}
	return config, nil
}
