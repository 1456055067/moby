// Package ipamutils provides utility functions for ipam management
package ipamutils

import (
	"net/netip"
	"slices"
)

var (
	localScopeDefaultNetworks = []*NetworkToSplit{
		{netip.MustParsePrefix("172.17.0.0/16"), 16},
		{netip.MustParsePrefix("172.18.0.0/16"), 16},
		{netip.MustParsePrefix("172.19.0.0/16"), 16},
		{netip.MustParsePrefix("172.20.0.0/14"), 16},
		{netip.MustParsePrefix("172.24.0.0/14"), 16},
		{netip.MustParsePrefix("172.28.0.0/14"), 16},
		{netip.MustParsePrefix("192.168.0.0/16"), 20},
	}
	globalScopeDefaultNetworks = []*NetworkToSplit{
		{netip.MustParsePrefix("10.0.0.0/8"), 24},
	}

	// IPv6 Unique Local Addresses (ULA)(RFC 4193) for local scope networks.
	localScopeDefaultNetworksV6 = []*NetworkToSplit{
		{netip.MustParsePrefix("fd00::/48"), 64}, // ~65k /64 subnets, each with over 18 quintillion addresses
	}

	// IPv6 Unique Local Addresses for global scope networks.
	globalScopeDefaultNetworksV6 = []*NetworkToSplit{
		{netip.MustParsePrefix("fd00:0:1::/48"), 64},
	}
)

// NetworkToSplit represent a network that has to be split in chunks with mask length Size.
// Each subnet in the set is derived from the Base pool. Base is to be passed
// in CIDR format.
// Example: a Base "10.10.0.0/16 with Size 24 will define the set of 256
// 10.10.[0-255].0/24 address pools
type NetworkToSplit struct {
	Base netip.Prefix `json:"base"`
	Size int          `json:"size"`
}

// FirstPrefix returns the first prefix available in NetworkToSplit.
func (n NetworkToSplit) FirstPrefix() netip.Prefix {
	return netip.PrefixFrom(n.Base.Addr(), n.Size)
}

// Overlaps is a util function checking whether 'p' overlaps with 'n'.
func (n NetworkToSplit) Overlaps(p netip.Prefix) bool {
	return n.Base.Overlaps(p)
}

// GetGlobalScopeDefaultNetworks returns a copy of the global-scope network list.
func GetGlobalScopeDefaultNetworks() []*NetworkToSplit {
	return slices.Clone(globalScopeDefaultNetworks)
}

// GetLocalScopeDefaultNetworks returns a copy of the default local-scope network list.
func GetLocalScopeDefaultNetworks() []*NetworkToSplit {
	return slices.Clone(localScopeDefaultNetworks)
}

// GetLocalScopeDefaultNetworksV6 returns a copy of the default local-scope IPv6 network list.
func GetLocalScopeDefaultNetworksV6() []*NetworkToSplit {
	return slices.Clone(localScopeDefaultNetworksV6)
}

// GetGlobalScopeDefaultNetworksV6 returns a copy of the global-scope IPv6 network list.
func GetGlobalScopeDefaultNetworksV6() []*NetworkToSplit {
	return slices.Clone(globalScopeDefaultNetworksV6)
}
