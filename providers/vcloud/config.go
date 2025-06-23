/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package vcloud

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

// VCloudConfig holds the configuration for the VCloud provider
type VCloudConfig struct {
	ClusterID     string
	ClusterName   string
	MgmtURL       string
	ProviderToken string
}

// readConfig reads the cloud configuration from the specified reader
func readConfig(config io.Reader) (*VCloudConfig, error) {
	if config == nil {
		return nil, fmt.Errorf("no vcloud config provided")
	}

	cfg := &VCloudConfig{}
	scanner := bufio.NewScanner(config)
	inVCloudSection := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		// Check for section header
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			sectionName := strings.TrimSpace(line[1 : len(line)-1])
			inVCloudSection = strings.EqualFold(sectionName, "vCloud")
			continue
		}

		// Parse key-value pairs in vCloud section
		if inVCloudSection && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}

			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			switch key {
			case "CLUSTER_ID":
				cfg.ClusterID = value
			case "CLUSTER_NAME":
				cfg.ClusterName = value
			case "MGMT_URL":
				cfg.MgmtURL = value
			case "PROVIDER_TOKEN":
				cfg.ProviderToken = value
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading config: %v", err)
	}

	return cfg, nil
}

// validateConfig validates the VCloud configuration
func validateConfig(cfg *VCloudConfig) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	// Validate required fields
	if cfg.ClusterID == "" {
		return fmt.Errorf("CLUSTER_ID is required")
	}

	if cfg.ClusterName == "" {
		return fmt.Errorf("CLUSTER_NAME is required")
	}

	if cfg.MgmtURL == "" {
		return fmt.Errorf("MGMT_URL is required")
	}

	if cfg.ProviderToken == "" {
		return fmt.Errorf("PROVIDER_TOKEN is required")
	}

	// Validate CLUSTER_ID is a valid UUID
	if _, err := uuid.Parse(cfg.ClusterID); err != nil {
		return fmt.Errorf("CLUSTER_ID must be a valid UUID: %v", err)
	}

	// Validate MGMT_URL is a valid URL
	if _, err := url.Parse(cfg.MgmtURL); err != nil {
		return fmt.Errorf("MGMT_URL must be a valid URL: %v", err)
	}

	return nil
}
