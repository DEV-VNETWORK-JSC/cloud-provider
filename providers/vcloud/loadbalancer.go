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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	cloudprovider "k8s.io/cloud-provider"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

// VCloudLoadBalancer implements the LoadBalancer interface for VCloud
type VCloudLoadBalancer struct {
	provider *VCloudProvider
}

// LoadBalancerRequest represents a request to create/update a load balancer
type LoadBalancerRequest struct {
	Name      string             `json:"name"`
	Ports     []LoadBalancerPort `json:"ports"`
	Nodes     []string           `json:"nodes"`
	Namespace string             `json:"namespace"`
	Type      string             `json:"type"`
}

// LoadBalancerPort represents a port configuration for the load balancer
type LoadBalancerPort struct {
	Name        string `json:"name"`
	Port        int32  `json:"port"`
	TargetPort  string `json:"targetPort"`
	Protocol    string `json:"protocol"`
	NodePort    int32  `json:"nodePort,omitempty"`
	AppProtocol string `json:"appProtocol,omitempty"`
}

// LoadBalancerResponse represents the API response for load balancer operations
type LoadBalancerResponse struct {
	Status int `json:"status"`
	Data   struct {
		Ingress []struct {
			IP string `json:"ip"`
		} `json:"ingress"`
	} `json:"data"`
}

// NewVCloudLoadBalancer creates a new VCloudLoadBalancer instance
func NewVCloudLoadBalancer(provider *VCloudProvider) cloudprovider.LoadBalancer {
	return &VCloudLoadBalancer{
		provider: provider,
	}
}

// GetLoadBalancer returns the load balancer status
func (lb *VCloudLoadBalancer) GetLoadBalancer(ctx context.Context, clusterName string, service *v1.Service) (status *v1.LoadBalancerStatus, exists bool, err error) {
	lbName := lb.GetLoadBalancerName(ctx, clusterName, service)
	klog.V(4).Infof("Getting load balancer %s", lbName)

	path := fmt.Sprintf("/ingresses/%s", lbName)
	resp, err := lb.provider.Request(ctx, "GET", path, nil)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get load balancer %s: %v", lbName, err)
	}
	defer resp.Body.Close()

	// Handle 404 - load balancer doesn't exist
	if resp.StatusCode == 404 {
		klog.V(4).Infof("Load balancer %s not found", lbName)
		return nil, false, nil
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, false, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var lbResp LoadBalancerResponse
	if err := json.NewDecoder(resp.Body).Decode(&lbResp); err != nil {
		return nil, false, fmt.Errorf("failed to decode response: %v", err)
	}

	// Build status
	status = &v1.LoadBalancerStatus{}
	for _, ingress := range lbResp.Data.Ingress {
		status.Ingress = append(status.Ingress, v1.LoadBalancerIngress{
			IP: ingress.IP,
		})
	}

	return status, true, nil
}

// GetLoadBalancerName returns the name of the load balancer
func (lb *VCloudLoadBalancer) GetLoadBalancerName(ctx context.Context, clusterName string, service *v1.Service) string {
	// Format: {cluster-name}-ingress-{uid-prefix}-{service-name}
	uid := strings.Split(string(service.UID), "-")
	lbName := fmt.Sprintf("%s-ingress-%s", lb.provider.clusterName, uid[0])
	return fmt.Sprintf("%s-%s", lbName, service.Name)
}

// EnsureLoadBalancer creates a new load balancer or updates an existing one
func (lb *VCloudLoadBalancer) EnsureLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) (*v1.LoadBalancerStatus, error) {
	lbName := lb.GetLoadBalancerName(ctx, clusterName, service)
	klog.V(2).Infof("Ensuring load balancer %s for service %s/%s", lbName, service.Namespace, service.Name)

	// Build request
	req := lb.buildLoadBalancerRequest(lbName, service, nodes)

	// Marshal request
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	// Make request
	resp, err := lb.provider.Request(ctx, "POST", "/ingresses", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create/update load balancer: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var lbResp LoadBalancerResponse
	if err := json.NewDecoder(resp.Body).Decode(&lbResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	// Build status
	status := &v1.LoadBalancerStatus{}
	for _, ingress := range lbResp.Data.Ingress {
		status.Ingress = append(status.Ingress, v1.LoadBalancerIngress{
			IP: ingress.IP,
		})
	}

	klog.V(2).Infof("Successfully ensured load balancer %s", lbName)
	return status, nil
}

// UpdateLoadBalancer updates the nodes serving the load balancer
func (lb *VCloudLoadBalancer) UpdateLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) error {
	lbName := lb.GetLoadBalancerName(ctx, clusterName, service)
	klog.V(2).Infof("Updating load balancer %s", lbName)

	// Build update request
	req := lb.buildLoadBalancerRequest(lbName, service, nodes)

	// Marshal request
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %v", err)
	}

	// Make request
	path := fmt.Sprintf("/ingresses/%s", lbName)
	resp, err := lb.provider.Request(ctx, "PUT", path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to update load balancer: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	klog.V(2).Infof("Successfully updated load balancer %s", lbName)
	return nil
}

// EnsureLoadBalancerDeleted deletes the load balancer
func (lb *VCloudLoadBalancer) EnsureLoadBalancerDeleted(ctx context.Context, clusterName string, service *v1.Service) error {
	lbName := lb.GetLoadBalancerName(ctx, clusterName, service)
	klog.V(2).Infof("Deleting load balancer %s", lbName)

	path := fmt.Sprintf("/ingresses/%s", lbName)
	resp, err := lb.provider.Request(ctx, "DELETE", path, nil)
	if err != nil {
		return fmt.Errorf("failed to delete load balancer: %v", err)
	}
	defer resp.Body.Close()

	// 404 is OK - already deleted
	if resp.StatusCode == 404 {
		klog.V(4).Infof("Load balancer %s already deleted", lbName)
		return nil
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	klog.V(2).Infof("Successfully deleted load balancer %s", lbName)
	return nil
}

// buildLoadBalancerRequest builds a load balancer request from service and nodes
func (lb *VCloudLoadBalancer) buildLoadBalancerRequest(name string, service *v1.Service, nodes []*v1.Node) *LoadBalancerRequest {
	// Extract node IPs
	nodeIPs := make([]string, 0, len(nodes))
	for _, node := range nodes {
		for _, addr := range node.Status.Addresses {
			if addr.Type == v1.NodeInternalIP {
				nodeIPs = append(nodeIPs, addr.Address)
				break
			}
		}
	}

	// Build ports
	ports := make([]LoadBalancerPort, 0, len(service.Spec.Ports))
	for _, svcPort := range service.Spec.Ports {
		port := LoadBalancerPort{
			Name:       svcPort.Name,
			Port:       svcPort.Port,
			TargetPort: svcPort.TargetPort.String(),
			Protocol:   string(svcPort.Protocol),
			NodePort:   svcPort.NodePort,
		}

		// Auto-generate port name if empty
		if port.Name == "" {
			port.Name = fmt.Sprintf("port-%d", svcPort.Port)
		}

		// Set app protocol if available
		if svcPort.AppProtocol != nil {
			port.AppProtocol = *svcPort.AppProtocol
		}

		ports = append(ports, port)
	}

	return &LoadBalancerRequest{
		Name:      name,
		Ports:     ports,
		Nodes:     nodeIPs,
		Namespace: service.Namespace,
		Type:      string(service.Spec.Type),
	}
}
