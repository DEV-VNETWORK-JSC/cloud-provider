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
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"
)

const (
	ProviderName = "vcloud"

	// HTTP client settings
	defaultTimeout = 60 * time.Second
	maxRetries     = 3

	// Cache TTL settings
	instanceCacheTTL    = 30 * time.Second
	nonExistentCacheTTL = 5 * time.Second
)

// VCloudProvider implements the cloud provider interface for VCloud
type VCloudProvider struct {
	clusterName   string
	clusterID     string
	mgmtURL       string
	providerToken string
	httpClient    *http.Client

	// Sub-interfaces
	instances    cloudprovider.InstancesV2
	loadbalancer cloudprovider.LoadBalancer
}

// init registers the VCloud provider with the cloud provider framework
func init() {
	cloudprovider.RegisterCloudProvider(ProviderName, func(config io.Reader) (cloudprovider.Interface, error) {
		return NewVCloudProvider(config)
	})
}

// NewVCloudProvider creates a new instance of the VCloud cloud provider
func NewVCloudProvider(config io.Reader) (cloudprovider.Interface, error) {
	cfg, err := readConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to read vcloud config: %v", err)
	}

	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid vcloud config: %v", err)
	}

	provider := &VCloudProvider{
		clusterName:   cfg.ClusterName,
		clusterID:     cfg.ClusterID,
		mgmtURL:       cfg.MgmtURL,
		providerToken: cfg.ProviderToken,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}

	// Initialize sub-interfaces
	provider.instances = NewVCloudInstances(provider)
	provider.loadbalancer = NewVCloudLoadBalancer(provider)

	klog.Infof("VCloud provider initialized with cluster %s (ID: %s)", provider.clusterName, provider.clusterID)
	return provider, nil
}

// Initialize provides the cloud with a kubernetes client builder
func (p *VCloudProvider) Initialize(clientBuilder cloudprovider.ControllerClientBuilder, stop <-chan struct{}) {
	klog.V(3).Infof("Initializing VCloud provider")
}

// LoadBalancer returns a LoadBalancer interface if supported
func (p *VCloudProvider) LoadBalancer() (cloudprovider.LoadBalancer, bool) {
	return p.loadbalancer, true
}

// Instances returns an Instances interface if supported
func (p *VCloudProvider) Instances() (cloudprovider.Instances, bool) {
	return nil, false
}

// InstancesV2 returns an InstancesV2 interface if supported
func (p *VCloudProvider) InstancesV2() (cloudprovider.InstancesV2, bool) {
	return p.instances, true
}

// Zones returns a Zones interface if supported
func (p *VCloudProvider) Zones() (cloudprovider.Zones, bool) {
	return nil, false
}

// Clusters returns a Clusters interface if supported
func (p *VCloudProvider) Clusters() (cloudprovider.Clusters, bool) {
	return nil, false
}

// Routes returns a Routes interface if supported
func (p *VCloudProvider) Routes() (cloudprovider.Routes, bool) {
	return nil, false
}

// ProviderName returns the cloud provider ID
func (p *VCloudProvider) ProviderName() string {
	return ProviderName
}

// HasClusterID returns true if the cluster has a clusterID
func (p *VCloudProvider) HasClusterID() bool {
	return p.clusterID != ""
}

// Request makes an HTTP request to the VCloud API with retry logic
func (p *VCloudProvider) Request(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	// Construct the full URL
	url := p.mgmtURL
	if !strings.Contains(p.mgmtURL, "/clusters/") {
		url = fmt.Sprintf("%s/clusters/%s", p.mgmtURL, p.clusterID)
	}
	if path != "" {
		url = fmt.Sprintf("%s%s", url, path)
	}

	var resp *http.Response
	var err error

	for i := 0; i < maxRetries; i++ {
		req, err := http.NewRequestWithContext(ctx, method, url, body)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %v", err)
		}

		// Set headers
		req.Header.Set("X-Provider-Token", p.providerToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err = p.httpClient.Do(req)
		if err != nil {
			klog.V(4).Infof("Request failed (attempt %d/%d): %v", i+1, maxRetries, err)
			if i < maxRetries-1 {
				time.Sleep(time.Duration(i+1) * time.Second)
				continue
			}
			return nil, err
		}

		// Check if we need to retry based on status code
		if resp.StatusCode >= 500 && i < maxRetries-1 {
			resp.Body.Close()
			klog.V(4).Infof("Server error %d, retrying (attempt %d/%d)", resp.StatusCode, i+1, maxRetries)
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}

		return resp, nil
	}

	return resp, err
}
