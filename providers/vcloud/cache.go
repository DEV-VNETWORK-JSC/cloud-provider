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
	"sync"
	"time"

	"k8s.io/klog/v2"
)

// cacheEntry represents a cached instance info entry
type cacheEntry struct {
	info      *InstanceInfo
	timestamp time.Time
	ttl       time.Duration
}

// instanceCache provides thread-safe caching for instance information
type instanceCache struct {
	mu       sync.RWMutex
	cache    map[string]*cacheEntry
	provider *VCloudProvider
}

// newInstanceCache creates a new instance cache
func newInstanceCache(provider *VCloudProvider) *instanceCache {
	return &instanceCache{
		cache:    make(map[string]*cacheEntry),
		provider: provider,
	}
}

// get retrieves instance info from cache or fetches from API
func (c *instanceCache) get(ctx context.Context, instanceID string) (*InstanceInfo, error) {
	klog.V(4).Infof("Cache.get: looking up instance %s", instanceID)

	// Try to get from cache first
	c.mu.RLock()
	entry, exists := c.cache[instanceID]
	c.mu.RUnlock()

	if exists && !c.isExpired(entry) {
		klog.V(4).Infof("Cache.get: cache hit for instance %s (exists=%t)", instanceID, entry.info.Exists)
		return entry.info, nil
	}

	if exists {
		klog.V(4).Infof("Cache.get: cache entry expired for instance %s, refetching", instanceID)
	} else {
		klog.V(4).Infof("Cache.get: cache miss for instance %s, fetching from API", instanceID)
	}

	// Use the VCloudInstances type to access GetInstanceInfo
	instances := &VCloudInstances{provider: c.provider, cache: c}
	info, err := instances.GetInstanceInfo(ctx, instanceID)
	if err != nil {
		klog.Errorf("Cache.get: failed to get instance info from API for %s: %v", instanceID, err)
		return nil, err
	}

	// Determine TTL based on instance existence
	ttl := instanceCacheTTL
	if !info.Exists {
		ttl = nonExistentCacheTTL
		klog.V(3).Infof("Cache.get: instance %s does not exist, using shorter TTL (%v)", instanceID, ttl)
	} else {
		klog.V(4).Infof("Cache.get: instance %s exists, using normal TTL (%v)", instanceID, ttl)
	}

	// Update cache
	c.mu.Lock()
	c.cache[instanceID] = &cacheEntry{
		info:      info,
		timestamp: time.Now(),
		ttl:       ttl,
	}

	// Clean up old entries if cache is getting large
	if len(c.cache) > 100 {
		klog.V(4).Infof("Cache.get: cache size (%d) exceeded limit, cleaning up", len(c.cache))
		c.cleanupOldEntriesLocked()
	}
	c.mu.Unlock()

	klog.V(4).Infof("Cache.get: cached instance %s info (exists=%t)", instanceID, info.Exists)
	return info, nil
}

// isExpired checks if a cache entry is expired
func (c *instanceCache) isExpired(entry *cacheEntry) bool {
	return time.Since(entry.timestamp) > entry.ttl
}

// cleanupOldEntriesLocked removes expired entries (must be called with write lock held)
func (c *instanceCache) cleanupOldEntriesLocked() {
	klog.V(4).Info("Cleaning up expired cache entries")

	for id, entry := range c.cache {
		if c.isExpired(entry) {
			delete(c.cache, id)
		}
	}
}

// invalidate removes an entry from the cache
func (c *instanceCache) invalidate(instanceID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.cache, instanceID)
	klog.V(5).Infof("Invalidated cache entry for instance %s", instanceID)
}

// clear removes all entries from the cache
func (c *instanceCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[string]*cacheEntry)
	klog.V(4).Info("Cleared all cache entries")
}
