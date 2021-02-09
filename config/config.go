// Config must support the following parameters
// targetAnnotation - the annotation a pvc must have to attach a snapshot policy to the associated disk
// googleProject - project id hosting cluster and pds
// zone - gcp zone resources live in
// region - gcp region resources live in
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
