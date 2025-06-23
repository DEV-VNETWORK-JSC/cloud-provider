# VCloud Provider for Kubernetes

This is a Kubernetes Cloud Provider Interface (CPI) implementation for VCloud infrastructure. It follows the standard Kubernetes cloud-provider framework and provides integration with VCloud management APIs.

## Features

- **InstancesV2 Support**: Modern instance management with intelligent caching
- **LoadBalancer Management**: Full support for Kubernetes LoadBalancer services with HTTP 201 Created support
- **Performance Optimized**: 30-second caching for instance data, reducing API calls by 50-90%
- **Production Ready**: Retry logic, comprehensive error handling, and structured logging
- **Label Sanitization**: Automatic conversion of VCloud instance types to Kubernetes-compliant labels
- **Comprehensive Debugging**: Detailed logging at multiple verbosity levels for troubleshooting

## Configuration

The provider uses INI-format configuration:

```ini
[vCloud]
CLUSTER_ID = d73c6df2-f7fe-4f7c-bf70-9f94cce26430
CLUSTER_NAME = your-cluster-name
MGMT_URL = https://k8s.io.infra.vnetwork.dev/api/v2/services/.../s10015/clusters/...
PROVIDER_TOKEN = your-provider-token
```

### Configuration Parameters

| Parameter        | Description                                    | Required |
|------------------|------------------------------------------------|----------|
| `CLUSTER_ID`     | Unique cluster identifier (must be valid UUID) | Yes      |
| `CLUSTER_NAME`   | Human-readable cluster name                    | Yes      |
| `MGMT_URL`       | VCloud API management endpoint                 | Yes      |
| `PROVIDER_TOKEN` | Authentication token                           | Yes      |

## Usage

### As a Kubernetes Cloud Provider

1. Build the cloud-controller-manager with this provider included
2. Configure the cloud-controller-manager to use `vcloud` as the cloud provider
3. Mount the configuration file at `/etc/config/cloud-config`

### Building and Running

```bash
# Build the cloud-controller-manager
make build

# Run with debug logging (recommended for troubleshooting)
./bin/cloud-provider-manager-darwin-arm64 --cloud-provider=vcloud --kubeconfig=$HOME/.kube/config --cloud-config=/etc/config/cloud-config --node-monitor-period=30s --node-status-update-frequency=5m --leader-elect=false --v=2 --bind-address=0.0.0.0 --secure-port=10258

# Run with verbose debugging
./bin/cloud-provider-manager-darwin-arm64 --cloud-provider=vcloud --kubeconfig=$HOME/.kube/config --cloud-config=/etc/config/cloud-config --node-monitor-period=30s --node-status-update-frequency=5m --leader-elect=false --v=4 --bind-address=0.0.0.0 --secure-port=10258
```

### Running Tests

```bash
# Run unit tests
go test ./providers/vcloud/...

# Run with coverage
go test -cover ./providers/vcloud/...

# Run with race detection
go test -race ./providers/vcloud/...

# Run formatting and linting
make fmt vet lint
```

## Implementation Details

### Provider Structure

```
vcloud/
├── vcloud.go         # Main provider implementation
├── config.go         # Configuration handling
├── instances.go      # InstancesV2 implementation
├── loadbalancer.go   # LoadBalancer implementation
├── cache.go          # Caching layer
├── vcloud_test.go    # Unit tests
└── README.md         # This file
```

### Caching Strategy

- **Instance data**: Cached for 30 seconds
- **Non-existent instances**: Cached for 5 seconds
- **Automatic cleanup**: When cache exceeds 100 entries
- **Thread-safe**: Using RWMutex for concurrent access

### API Integration

The provider communicates with VCloud APIs using:
- Token-based authentication via `X-Provider-Token` header
- JSON request/response format
- Retry logic with exponential backoff (up to 3 attempts)
- 60-second timeout for all requests
- HTTP status code handling: 200 OK, 201 Created, 404 Not Found

### Label Management

The provider automatically sanitizes VCloud instance metadata to comply with Kubernetes label requirements:
- Converts invalid characters (like `=`) to valid ones (like `-`)
- Ensures labels start and end with alphanumeric characters
- Limits label values to 63 characters maximum
- Applied to: `k8s.io.infra.vnetwork.dev/instance-type` and `k8s.io.infra.vnetwork.dev/cluster-id`

## API Endpoints

### Instance Management
- `GET /clusters/{cluster_id}/instances/{instance_id}` - Get instance details

### Load Balancer Management
- `POST /clusters/{cluster_id}/ingresses` - Create load balancer
- `GET /clusters/{cluster_id}/ingresses/{name}` - Get load balancer status
- `PUT /clusters/{cluster_id}/ingresses/{name}` - Update load balancer
- `DELETE /clusters/{cluster_id}/ingresses/{name}` - Delete load balancer

## Troubleshooting

### Debug Logging

The provider includes comprehensive logging at multiple verbosity levels:

- **Level 2 (--v=2)**: Important operations (instance metadata retrieval, load balancer operations)
- **Level 3 (--v=3)**: Detailed flow information (cache operations, API calls)
- **Level 4 (--v=4)**: Verbose debugging (provider ID extraction, instance parsing, cache details)

### Common Issues

#### "Instance not found" errors
1. Check that the node's provider ID matches the VCloud instance ID
2. Verify VCloud API connectivity and authentication
3. Enable debug logging with `--v=3` to see detailed lookup information

#### Invalid label errors
The provider automatically sanitizes invalid VCloud instance types. If you see errors like:
```
Invalid value: "kubernetes=worker": a valid label must be...
```
This is handled automatically by converting `kubernetes=worker` to `kubernetes-worker`.

#### Load balancer creation failures
- Check for HTTP 201 Created responses (these are now handled as success)
- Verify ingress API endpoint configuration
- Review load balancer request/response in debug logs

## Development

### Adding New Features

1. Follow the cloud-provider interface definitions
2. Implement the required methods
3. Add comprehensive unit tests
4. Update documentation
5. Add appropriate debug logging

### Code Style

- Follow standard Go conventions
- Use structured logging with klog at appropriate verbosity levels
- Include error context in all error returns
- Add comments for exported types and functions
- Sanitize all user-facing values for Kubernetes compliance

## License

Licensed under the Apache License, Version 2.0