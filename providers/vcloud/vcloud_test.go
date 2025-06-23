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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestNewVCloudProvider(t *testing.T) {
	tests := []struct {
		name      string
		config    string
		wantErr   bool
		errString string
	}{
		{
			name: "valid config",
			config: `[vCloud]
CLUSTER_ID = d73c6df2-f7fe-4f7c-bf70-9f94cce26430
CLUSTER_NAME = test-cluster
MGMT_URL = https://api.vcloud.example.com
PROVIDER_TOKEN = test-token`,
			wantErr: false,
		},
		{
			name:      "nil config",
			config:    "",
			wantErr:   true,
			errString: "no vcloud config provided",
		},
		{
			name: "missing cluster ID",
			config: `[vCloud]
CLUSTER_NAME = test-cluster
MGMT_URL = https://api.vcloud.example.com
PROVIDER_TOKEN = test-token`,
			wantErr:   true,
			errString: "CLUSTER_ID is required",
		},
		{
			name: "invalid cluster ID",
			config: `[vCloud]
CLUSTER_ID = invalid-uuid
CLUSTER_NAME = test-cluster
MGMT_URL = https://api.vcloud.example.com
PROVIDER_TOKEN = test-token`,
			wantErr:   true,
			errString: "CLUSTER_ID must be a valid UUID",
		},
		{
			name: "invalid URL",
			config: `[vCloud]
CLUSTER_ID = d73c6df2-f7fe-4f7c-bf70-9f94cce26430
CLUSTER_NAME = test-cluster
MGMT_URL = not-a-url
PROVIDER_TOKEN = test-token`,
			wantErr:   true,
			errString: "MGMT_URL must be a valid URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reader *strings.Reader
			if tt.config != "" {
				reader = strings.NewReader(tt.config)
			}

			provider, err := NewVCloudProvider(reader)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				} else if tt.errString != "" && !strings.Contains(err.Error(), tt.errString) {
					t.Errorf("expected error containing %q, got %q", tt.errString, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if provider == nil {
					t.Error("expected provider, got nil")
				}
			}
		})
	}
}

func TestProviderInterface(t *testing.T) {
	provider := createTestProvider(t)

	// Test ProviderName
	if name := provider.ProviderName(); name != ProviderName {
		t.Errorf("expected provider name %q, got %q", ProviderName, name)
	}

	// Test HasClusterID
	if !provider.HasClusterID() {
		t.Error("expected HasClusterID to return true")
	}

	// Test LoadBalancer support
	if lb, ok := provider.LoadBalancer(); !ok || lb == nil {
		t.Error("expected LoadBalancer to be supported")
	}

	// Test InstancesV2 support
	if instances, ok := provider.InstancesV2(); !ok || instances == nil {
		t.Error("expected InstancesV2 to be supported")
	}

	// Test unsupported interfaces
	if _, ok := provider.Instances(); ok {
		t.Error("expected Instances to not be supported")
	}

	if _, ok := provider.Routes(); ok {
		t.Error("expected Routes to not be supported")
	}

	if _, ok := provider.Zones(); ok {
		t.Error("expected Zones to not be supported")
	}

	if _, ok := provider.Clusters(); ok {
		t.Error("expected Clusters to not be supported")
	}
}

func TestGetProviderID(t *testing.T) {
	provider := createTestProvider(t)
	instances := &VCloudInstances{provider: provider}

	tests := []struct {
		name     string
		node     *v1.Node
		expected string
	}{
		{
			name: "provider ID with scheme",
			node: &v1.Node{
				Spec: v1.NodeSpec{
					ProviderID: "vcloud://instance-123",
				},
			},
			expected: "instance-123",
		},
		{
			name: "provider ID without scheme",
			node: &v1.Node{
				Spec: v1.NodeSpec{
					ProviderID: "instance-456",
				},
			},
			expected: "instance-456",
		},
		{
			name: "fallback to node name",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-789",
				},
			},
			expected: "node-789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := instances.getProviderID(tt.node)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestInstanceShutdownStates(t *testing.T) {
	tests := []struct {
		state    string
		shutdown bool
	}{
		{"POWERED_ON", false},
		{"POWERED_OFF", true},
		{"SUSPENDED", true},
		{"TERMINATED", true},
		{"BACKUP", true},
		{"BACKUP_POWEROFF", true},
		{"RUNNING", false},
		{"PENDING", false},
		{"UNKNOWN", false},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			result := isInstanceShutdown(tt.state)
			if result != tt.shutdown {
				t.Errorf("expected %v for state %q, got %v", tt.shutdown, tt.state, result)
			}
		})
	}
}

func TestLoadBalancerName(t *testing.T) {
	provider := createTestProvider(t)
	lb := &VCloudLoadBalancer{provider: provider}

	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-service",
			UID:  types.UID("abc123-def456-ghi789"),
		},
	}

	name := lb.GetLoadBalancerName(context.Background(), "cluster", service)
	expected := "test-cluster-ingress-abc123-test-service"

	if name != expected {
		t.Errorf("expected load balancer name %q, got %q", expected, name)
	}
}

func TestBuildLoadBalancerRequest(t *testing.T) {
	provider := createTestProvider(t)
	lb := &VCloudLoadBalancer{provider: provider}

	appProtocol := "http"
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Type: v1.ServiceTypeLoadBalancer,
			Ports: []v1.ServicePort{
				{
					Name:        "http",
					Port:        80,
					TargetPort:  intstrFromInt(8080),
					Protocol:    v1.ProtocolTCP,
					NodePort:    30080,
					AppProtocol: &appProtocol,
				},
				{
					Port:       443,
					TargetPort: intstrFromInt(8443),
					Protocol:   v1.ProtocolTCP,
					NodePort:   30443,
				},
			},
		},
	}

	nodes := []*v1.Node{
		{
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{
					{Type: v1.NodeInternalIP, Address: "10.0.1.100"},
					{Type: v1.NodeExternalIP, Address: "1.2.3.4"},
				},
			},
		},
		{
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{
					{Type: v1.NodeInternalIP, Address: "10.0.1.101"},
				},
			},
		},
	}

	req := lb.buildLoadBalancerRequest("test-lb", service, nodes)

	// Verify basic fields
	if req.Name != "test-lb" {
		t.Errorf("expected name %q, got %q", "test-lb", req.Name)
	}

	if req.Namespace != "default" {
		t.Errorf("expected namespace %q, got %q", "default", req.Namespace)
	}

	if req.Type != "LoadBalancer" {
		t.Errorf("expected type %q, got %q", "LoadBalancer", req.Type)
	}

	// Verify nodes
	expectedNodes := []string{"10.0.1.100", "10.0.1.101"}
	if len(req.Nodes) != len(expectedNodes) {
		t.Errorf("expected %d nodes, got %d", len(expectedNodes), len(req.Nodes))
	}

	// Verify ports
	if len(req.Ports) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(req.Ports))
	}

	// Check first port
	if req.Ports[0].Name != "http" {
		t.Errorf("expected port name %q, got %q", "http", req.Ports[0].Name)
	}

	if req.Ports[0].AppProtocol != "http" {
		t.Errorf("expected app protocol %q, got %q", "http", req.Ports[0].AppProtocol)
	}

	// Check auto-generated port name
	if req.Ports[1].Name != "port-443" {
		t.Errorf("expected auto-generated port name %q, got %q", "port-443", req.Ports[1].Name)
	}
}

// Helper functions

func createTestProvider(t *testing.T) *VCloudProvider {
	config := `[vCloud]
CLUSTER_ID = d73c6df2-f7fe-4f7c-bf70-9f94cce26430
CLUSTER_NAME = test-cluster
MGMT_URL = https://api.vcloud.example.com
PROVIDER_TOKEN = test-token`

	provider, err := NewVCloudProvider(strings.NewReader(config))
	if err != nil {
		t.Fatalf("failed to create test provider: %v", err)
	}

	return provider.(*VCloudProvider)
}

func intstrFromInt(val int) intstr.IntOrString {
	return intstr.FromInt(val)
}

// Integration test helpers

func createTestServer(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()

	// Instance endpoints
	mux.HandleFunc("/clusters/d73c6df2-f7fe-4f7c-bf70-9f94cce26430/instances/", func(w http.ResponseWriter, r *http.Request) {
		instanceID := strings.TrimPrefix(r.URL.Path, "/clusters/d73c6df2-f7fe-4f7c-bf70-9f94cce26430/instances/")

		if instanceID == "not-found" {
			w.WriteHeader(404)
			fmt.Fprintf(w, `{"status": 404, "error": "Instance not found"}`)
			return
		}

		w.WriteHeader(200)
		fmt.Fprintf(w, `{
			"status": 200,
			"data": {
				"instance": {
					"id": "%s",
					"name": "test-node",
					"state": "POWERED_ON",
					"status": "active",
					"zone": "zone-a",
					"type": "kubernetes=worker",
					"owned": true,
					"metadata": {
						"ip": "10.0.1.100",
						"flavor": "v2g-memory-4-8",
						"cluster": {
							"id": "d73c6df2-f7fe-4f7c-bf70-9f94cce26430",
							"zone": "zone-a",
							"tenant": "region-1"
						},
						"resources": {
							"cores": 4,
							"memory": 8192,
							"volumes": 1
						}
					}
				}
			}
		}`, instanceID)
	})

	// Load balancer endpoints
	mux.HandleFunc("/clusters/d73c6df2-f7fe-4f7c-bf70-9f94cce26430/ingresses", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.WriteHeader(200)
			fmt.Fprintf(w, `{
				"status": 200,
				"data": {
					"ingress": [{"ip": "203.0.113.10"}]
				}
			}`)
		}
	})

	return httptest.NewServer(mux)
}
