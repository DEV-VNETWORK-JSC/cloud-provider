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
	"encoding/json"
	"fmt"
	"io"
	"strings"

	v1 "k8s.io/api/core/v1"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"
)

// VCloudInstances implements the InstancesV2 interface for VCloud
type VCloudInstances struct {
	provider *VCloudProvider
	cache    *instanceCache
}

// Instance represents a VCloud instance
type Instance struct {
	Name     string `json:"name"`
	ID       string `json:"id"`
	UID      string `json:"uid"`
	Type     string `json:"type"`
	Zone     string `json:"zone"`
	Status   string `json:"status"`
	State    string `json:"state"`
	Owned    bool   `json:"owned"`
	Metadata struct {
		IP      string `json:"ip"`
		Flavor  string `json:"flavor"`
		Cluster struct {
			ID     string `json:"id"`
			Zone   string `json:"zone"`
			Tenant string `json:"tenant"`
		} `json:"cluster"`
		Resources struct {
			Cores   int `json:"cores"`
			Memory  int `json:"memory"`
			Volumes int `json:"volumes"`
		} `json:"resources"`
	} `json:"metadata"`
}

// InstanceInfo holds comprehensive instance information
type InstanceInfo struct {
	Exists      bool
	Shutdown    bool
	Metadata    *cloudprovider.InstanceMetadata
	RawInstance *Instance
}

// NewVCloudInstances creates a new VCloudInstances instance
func NewVCloudInstances(provider *VCloudProvider) cloudprovider.InstancesV2 {
	return &VCloudInstances{
		provider: provider,
		cache:    newInstanceCache(provider),
	}
}

// InstanceExists returns true if the instance for the given node exists
func (i *VCloudInstances) InstanceExists(ctx context.Context, node *v1.Node) (bool, error) {
	providerID := i.getProviderID(node)
	klog.V(3).Infof("InstanceExists: checking node %s with providerID=%s", node.Name, providerID)
	if providerID == "" {
		klog.Warningf("InstanceExists: provider ID is empty for node %s", node.Name)
		return false, fmt.Errorf("provider ID is empty for node %s", node.Name)
	}

	info, err := i.cache.get(ctx, providerID)
	if err != nil {
		klog.Errorf("InstanceExists: failed to get instance info for node %s (providerID=%s): %v", node.Name, providerID, err)
		return false, err
	}

	klog.V(3).Infof("InstanceExists: node %s (providerID=%s) exists=%t", node.Name, providerID, info.Exists)
	return info.Exists, nil
}

// InstanceShutdown returns true if the instance is shutdown
func (i *VCloudInstances) InstanceShutdown(ctx context.Context, node *v1.Node) (bool, error) {
	providerID := i.getProviderID(node)
	klog.V(3).Infof("InstanceShutdown: checking node %s with providerID=%s", node.Name, providerID)
	if providerID == "" {
		klog.Warningf("InstanceShutdown: provider ID is empty for node %s", node.Name)
		return false, fmt.Errorf("provider ID is empty for node %s", node.Name)
	}

	info, err := i.cache.get(ctx, providerID)
	if err != nil {
		klog.Errorf("InstanceShutdown: failed to get instance info for node %s (providerID=%s): %v", node.Name, providerID, err)
		return false, err
	}

	if !info.Exists {
		klog.Warningf("InstanceShutdown: instance not found for node %s (providerID=%s)", node.Name, providerID)
		return false, cloudprovider.InstanceNotFound
	}

	klog.V(3).Infof("InstanceShutdown: node %s (providerID=%s) shutdown=%t", node.Name, providerID, info.Shutdown)
	return info.Shutdown, nil
}

// InstanceMetadata returns the instance's metadata
func (i *VCloudInstances) InstanceMetadata(ctx context.Context, node *v1.Node) (*cloudprovider.InstanceMetadata, error) {
	providerID := i.getProviderID(node)
	klog.V(2).Infof("InstanceMetadata: getting metadata for node %s with providerID=%s", node.Name, providerID)
	if providerID == "" {
		klog.Warningf("InstanceMetadata: provider ID is empty for node %s", node.Name)
		return nil, fmt.Errorf("provider ID is empty for node %s", node.Name)
	}

	info, err := i.cache.get(ctx, providerID)
	if err != nil {
		klog.Errorf("InstanceMetadata: failed to get instance info for node %s (providerID=%s): %v", node.Name, providerID, err)
		return nil, err
	}

	if !info.Exists {
		klog.Warningf("InstanceMetadata: instance not found for node %s (providerID=%s)", node.Name, providerID)
		return nil, cloudprovider.InstanceNotFound
	}

	klog.V(3).Infof("InstanceMetadata: successfully retrieved metadata for node %s (providerID=%s)", node.Name, providerID)
	return info.Metadata, nil
}

// getProviderID extracts the provider ID from a node
func (i *VCloudInstances) getProviderID(node *v1.Node) string {
	if node.Spec.ProviderID != "" {
		// Handle both "provider://uuid" and "uuid" formats
		parts := strings.Split(node.Spec.ProviderID, "://")
		if len(parts) == 2 {
			klog.V(4).Infof("getProviderID: extracted provider ID %s from %s for node %s", parts[1], node.Spec.ProviderID, node.Name)
			return parts[1]
		}
		klog.V(4).Infof("getProviderID: using raw provider ID %s for node %s", node.Spec.ProviderID, node.Name)
		return node.Spec.ProviderID
	}

	// Fallback to node name if provider ID is not set
	klog.V(4).Infof("getProviderID: falling back to node name %s (no provider ID set)", node.Name)
	return node.Name
}

// GetInstanceInfo retrieves comprehensive instance information from the API
func (i *VCloudInstances) GetInstanceInfo(ctx context.Context, instanceID string) (*InstanceInfo, error) {
	path := fmt.Sprintf("/instances/%s", instanceID)
	klog.V(3).Infof("GetInstanceInfo: making API call for instance %s at path %s", instanceID, path)

	resp, err := i.provider.Request(ctx, "GET", path, nil)
	if err != nil {
		klog.Errorf("GetInstanceInfo: API request failed for instance %s: %v", instanceID, err)
		return nil, fmt.Errorf("failed to get instance %s: %v", instanceID, err)
	}
	defer resp.Body.Close()

	// Handle 404 - instance doesn't exist
	if resp.StatusCode == 404 {
		klog.Warningf("GetInstanceInfo: Instance %s not found (404) - API returned not found", instanceID)
		return &InstanceInfo{
			Exists: false,
		}, nil
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		klog.Errorf("GetInstanceInfo: unexpected status code %d for instance %s: %s", resp.StatusCode, instanceID, string(body))
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var apiResp struct {
		Status int `json:"status"`
		Data   struct {
			Instance Instance `json:"instance"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		klog.Errorf("GetInstanceInfo: failed to decode JSON response for instance %s: %v", instanceID, err)
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	instance := &apiResp.Data.Instance
	klog.V(4).Infof("GetInstanceInfo: parsed instance data for %s: Name=%s, Status=%s, State=%s", instanceID, instance.Name, instance.Status, instance.State)

	// Check if instance is terminated
	if instance.Status == "terminated" {
		klog.Infof("GetInstanceInfo: Instance %s is terminated (status=%s)", instanceID, instance.Status)
		return &InstanceInfo{
			Exists: false,
		}, nil
	}

	// Determine if instance is shutdown
	shutdown := isInstanceShutdown(instance.State)

	// Build metadata
	metadata := &cloudprovider.InstanceMetadata{
		ProviderID:   instanceID,
		InstanceType: instance.Metadata.Flavor,
		Zone:         instance.Zone,
		Region:       instance.Metadata.Cluster.Tenant,
		NodeAddresses: []v1.NodeAddress{
			{
				Type:    v1.NodeInternalIP,
				Address: instance.Metadata.IP,
			},
		},
	}

	// Add node labels
	metadata.AdditionalLabels = map[string]string{
		"k8s.io.infra.vnetwork.dev/instance-type": instance.Metadata.Flavor,
		"k8s.io.infra.vnetwork.dev/cluster-id":    instance.Metadata.Cluster.ID,
	}

	return &InstanceInfo{
		Exists:      true,
		Shutdown:    shutdown,
		Metadata:    metadata,
		RawInstance: instance,
	}, nil
}

// isInstanceShutdown determines if an instance is in shutdown state
func isInstanceShutdown(state string) bool {
	shutdownStates := map[string]bool{
		"POWERED_OFF":     true,
		"SUSPENDED":       true,
		"TERMINATED":      true,
		"BACKUP":          true,
		"BACKUP_POWEROFF": true,
	}

	return shutdownStates[state]
}
