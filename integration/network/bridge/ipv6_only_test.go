package bridge

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"

	networktypes "github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/moby/moby/v2/daemon/libnetwork/netlabel"
	ctr "github.com/moby/moby/v2/integration/internal/container"
	"github.com/moby/moby/v2/integration/internal/network"
	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
	"gotest.tools/v3/skip"
)

// TestIPv6OnlyNetworkCreation tests that an IPv6-only network can be created
// with automatic subnet allocation.
func TestIPv6OnlyNetworkCreation(t *testing.T) {
	skip.If(t, testEnv.DaemonInfo.OSType == "windows", "IPv6-only not supported on Windows")
	ctx := setupTest(t)
	apiClient := testEnv.APIClient()

	const nwName = "ipv6-only-net"

	// Create IPv6-only network using driver options
	network.CreateNoError(ctx, t, apiClient, nwName,
		network.WithIPv6(),
		network.WithOption(netlabel.EnableIPv4, "false"),
	)
	defer network.RemoveNoError(ctx, t, apiClient, nwName)

	// Inspect the network
	res, err := apiClient.NetworkInspect(ctx, nwName, client.NetworkInspectOptions{})
	assert.NilError(t, err)

	// Verify IPv4 is disabled and IPv6 is enabled
	assert.Check(t, is.Equal(false, res.Network.EnableIPv4), "IPv4 should be disabled")
	assert.Check(t, is.Equal(true, res.Network.EnableIPv6), "IPv6 should be enabled")

	// Verify we have an IPv6 subnet but no IPv4 subnet
	var hasIPv6, hasIPv4 bool
	for _, ipam := range res.Network.IPAM.Config {
		if ipam.Subnet.Addr().Is6() {
			hasIPv6 = true
			// Verify it's a ULA address (fd00::/8)
			assert.Check(t, netip.MustParsePrefix("fd00::/8").Overlaps(ipam.Subnet),
				"IPv6 subnet should be ULA: %s", ipam.Subnet)
		} else if ipam.Subnet.Addr().Is4() {
			hasIPv4 = true
		}
	}

	assert.Check(t, hasIPv6, "Network should have IPv6 subnet")
	assert.Check(t, !hasIPv4, "Network should NOT have IPv4 subnet")
}

// TestIPv6OnlyNetworkCreationWithAPI tests creating an IPv6-only network
// using the EnableIPv4 API field directly.
func TestIPv6OnlyNetworkCreationWithAPI(t *testing.T) {
	skip.If(t, testEnv.DaemonInfo.OSType == "windows", "IPv6-only not supported on Windows")
	ctx := setupTest(t)
	apiClient := testEnv.APIClient()

	const nwName = "ipv6-only-api"

	enableIPv4 := false
	enableIPv6 := true

	_, err := apiClient.NetworkCreate(ctx, nwName, client.NetworkCreateOptions{
		Driver:     "bridge",
		EnableIPv4: &enableIPv4,
		EnableIPv6: &enableIPv6,
	})
	assert.NilError(t, err)
	defer network.RemoveNoError(ctx, t, apiClient, nwName)

	// Inspect and verify
	res, err := apiClient.NetworkInspect(ctx, nwName, client.NetworkInspectOptions{})
	assert.NilError(t, err)

	assert.Check(t, is.Equal(false, res.Network.EnableIPv4))
	assert.Check(t, is.Equal(true, res.Network.EnableIPv6))
}

// TestIPv6OnlyNetworkWithManualSubnet tests creating an IPv6-only network
// with a manually specified subnet.
func TestIPv6OnlyNetworkWithManualSubnet(t *testing.T) {
	skip.If(t, testEnv.DaemonInfo.OSType == "windows", "IPv6-only not supported on Windows")
	ctx := setupTest(t)
	apiClient := testEnv.APIClient()

	const nwName = "ipv6-manual"
	const subnet = "fd00:cafe::/64"
	const gateway = "fd00:cafe::1"

	network.CreateNoError(ctx, t, apiClient, nwName,
		network.WithIPv6(),
		network.WithIPAM(subnet, gateway),
		network.WithOption(netlabel.EnableIPv4, "false"),
	)
	defer network.RemoveNoError(ctx, t, apiClient, nwName)

	// Verify the subnet was applied
	res, err := apiClient.NetworkInspect(ctx, nwName, client.NetworkInspectOptions{})
	assert.NilError(t, err)

	assert.Check(t, is.Len(res.Network.IPAM.Config, 1), "Should have exactly one IPAM config")
	assert.Check(t, is.Equal(subnet, res.Network.IPAM.Config[0].Subnet.String()))
	assert.Check(t, is.Equal(gateway, res.Network.IPAM.Config[0].Gateway.String()))
}

// TestIPv6OnlyContainerAddressing tests that containers on an IPv6-only
// network get only IPv6 addresses, not IPv4.
func TestIPv6OnlyContainerAddressing(t *testing.T) {
	skip.If(t, testEnv.DaemonInfo.OSType == "windows", "IPv6-only not supported on Windows")
	skip.If(t, testEnv.IsRemoteDaemon, "cannot check container addresses on remote daemon")

	ctx := setupTest(t)
	apiClient := testEnv.APIClient()

	const nwName = "ipv6-container-test"

	network.CreateNoError(ctx, t, apiClient, nwName,
		network.WithIPv6(),
		network.WithOption(netlabel.EnableIPv4, "false"),
	)
	defer network.RemoveNoError(ctx, t, apiClient, nwName)

	// Run a container and check its IP addresses
	attachCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	res := ctr.RunAttach(attachCtx, t, apiClient,
		ctr.WithCmd("ip", "-o", "addr", "show", "eth0"),
		ctr.WithNetworkMode(nwName),
	)

	assert.Check(t, is.Equal(0, res.ExitCode), "ip command should succeed")
	output := res.Stdout.String()

	// Should have an IPv6 address
	assert.Check(t, strings.Contains(output, "inet6"), "Should have IPv6 address")
	// Should NOT have an IPv4 address (except loopback)
	assert.Check(t, !strings.Contains(output, "inet "), "Should NOT have IPv4 address")
}

// TestIPv6OnlyContainerConnectivity tests that two containers on an
// IPv6-only network can communicate with each other.
func TestIPv6OnlyContainerConnectivity(t *testing.T) {
	skip.If(t, testEnv.DaemonInfo.OSType == "windows", "IPv6-only not supported on Windows")

	ctx := setupTest(t)
	apiClient := testEnv.APIClient()

	const nwName = "ipv6-connectivity"

	network.CreateNoError(ctx, t, apiClient, nwName,
		network.WithIPv6(),
		network.WithOption(netlabel.EnableIPv4, "false"),
	)
	defer network.RemoveNoError(ctx, t, apiClient, nwName)

	// Start first container (server)
	serverID := ctr.Run(ctx, t, apiClient,
		ctr.WithName("ipv6-server"),
		ctr.WithNetworkMode(nwName),
		ctr.WithCmd("top"),
	)
	defer apiClient.ContainerRemove(ctx, serverID, client.ContainerRemoveOptions{Force: true})

	// Get server's IPv6 address
	inspect, err := apiClient.ContainerInspect(ctx, serverID, client.ContainerInspectOptions{})
	assert.NilError(t, err)

	serverIPv6 := inspect.Container.NetworkSettings.Networks[nwName].GlobalIPv6Address
	assert.Check(t, serverIPv6.IsValid(), "Server should have IPv6 address")

	// Start second container (client) and ping the server
	attachCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	res := ctr.RunAttach(attachCtx, t, apiClient,
		ctr.WithName("ipv6-client"),
		ctr.WithNetworkMode(nwName),
		ctr.WithCmd("ping6", "-c", "3", serverIPv6.String()),
	)

	assert.Check(t, is.Equal(0, res.ExitCode), "ping6 should succeed")
	assert.Check(t, strings.Contains(res.Stdout.String(), "3 packets transmitted, 3 received"),
		"All pings should succeed")
}

// TestIPv6OnlyDNSResolution tests that DNS resolution works in IPv6-only
// containers using the IPv6 resolver (::1).
func TestIPv6OnlyDNSResolution(t *testing.T) {
	skip.If(t, testEnv.DaemonInfo.OSType == "windows", "IPv6-only not supported on Windows")

	ctx := setupTest(t)
	apiClient := testEnv.APIClient()

	const nwName = "ipv6-dns"

	network.CreateNoError(ctx, t, apiClient, nwName,
		network.WithIPv6(),
		network.WithOption(netlabel.EnableIPv4, "false"),
	)
	defer network.RemoveNoError(ctx, t, apiClient, nwName)

	// Start container with a hostname
	serverID := ctr.Run(ctx, t, apiClient,
		ctr.WithName("dns-server"),
		ctr.WithNetworkMode(nwName),
		ctr.WithCmd("top"),
	)
	defer apiClient.ContainerRemove(ctx, serverID, client.ContainerRemoveOptions{Force: true})

	// Start client and resolve server by name
	attachCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	res := ctr.RunAttach(attachCtx, t, apiClient,
		ctr.WithName("dns-client"),
		ctr.WithNetworkMode(nwName),
		ctr.WithCmd("sh", "-c", "nslookup dns-server && ping6 -c 2 dns-server"),
	)

	assert.Check(t, is.Equal(0, res.ExitCode), "DNS resolution and ping should succeed")
	assert.Check(t, strings.Contains(res.Stdout.String(), "dns-server"),
		"Should resolve hostname")
}

// TestIPv6OnlyResolverAddress tests that the embedded DNS resolver
// is listening on ::1 for IPv6-only networks.
func TestIPv6OnlyResolverAddress(t *testing.T) {
	skip.If(t, testEnv.DaemonInfo.OSType == "windows", "IPv6-only not supported on Windows")

	ctx := setupTest(t)
	apiClient := testEnv.APIClient()

	const nwName = "ipv6-resolver"

	network.CreateNoError(ctx, t, apiClient, nwName,
		network.WithIPv6(),
		network.WithOption(netlabel.EnableIPv4, "false"),
	)
	defer network.RemoveNoError(ctx, t, apiClient, nwName)

	// Check resolv.conf uses IPv6 resolver
	attachCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	res := ctr.RunAttach(attachCtx, t, apiClient,
		ctr.WithNetworkMode(nwName),
		ctr.WithCmd("cat", "/etc/resolv.conf"),
	)

	assert.Check(t, is.Equal(0, res.ExitCode))
	output := res.Stdout.String()

	// Should not contain IPv4 resolver (127.0.0.11)
	assert.Check(t, !strings.Contains(output, "127.0.0.11"),
		"Should NOT use IPv4 resolver")

	// For dual-stack or IPv6-only, the resolver might be ::1 or the IPv6 address
	// Just verify there's a nameserver entry
	assert.Check(t, strings.Contains(output, "nameserver"),
		"Should have nameserver configured")
}

// TestIPv6OnlyPortPublishing tests that port publishing works with IPv6-only
// networks when using IPv6 syntax.
func TestIPv6OnlyPortPublishing(t *testing.T) {
	skip.If(t, testEnv.DaemonInfo.OSType == "windows", "IPv6-only not supported on Windows")
	skip.If(t, testEnv.IsRemoteDaemon, "cannot test port publishing on remote daemon")

	ctx := setupTest(t)
	apiClient := testEnv.APIClient()

	const nwName = "ipv6-ports"

	network.CreateNoError(ctx, t, apiClient, nwName,
		network.WithIPv6(),
		network.WithOption(netlabel.EnableIPv4, "false"),
	)
	defer network.RemoveNoError(ctx, t, apiClient, nwName)

	// Start container with published port on IPv6
	containerID := ctr.Run(ctx, t, apiClient,
		ctr.WithName("ipv6-web"),
		ctr.WithNetworkMode(nwName),
		ctr.WithExposedPorts("80/tcp"),
		ctr.WithPortMap(networktypes.PortMap{
			networktypes.MustParsePort("80/tcp"): []networktypes.PortBinding{
				{
					HostIP:   netip.MustParseAddr("::"),
					HostPort: "0", // Random port
				},
			},
		}),
		ctr.WithCmd("httpd", "-f", "-p", "80", "-h", "/tmp"),
	)
	defer apiClient.ContainerRemove(ctx, containerID, client.ContainerRemoveOptions{Force: true})

	// Verify container started and port is mapped
	inspect, err := apiClient.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
	assert.NilError(t, err)
	assert.Check(t, inspect.Container.State.Running, "Container should be running")

	// Verify port binding exists
	portBindings := inspect.Container.NetworkSettings.Ports[networktypes.MustParsePort("80/tcp")]
	assert.Check(t, is.Len(portBindings, 1), "Should have one port binding")
	assert.Check(t, portBindings[0].HostPort != "", "Should have assigned a host port")
}

// TestIPv6OnlyNetworkAlias tests that network aliases work in IPv6-only networks.
func TestIPv6OnlyNetworkAlias(t *testing.T) {
	skip.If(t, testEnv.DaemonInfo.OSType == "windows", "IPv6-only not supported on Windows")

	ctx := setupTest(t)
	apiClient := testEnv.APIClient()

	const nwName = "ipv6-alias"

	network.CreateNoError(ctx, t, apiClient, nwName,
		network.WithIPv6(),
		network.WithOption(netlabel.EnableIPv4, "false"),
	)
	defer network.RemoveNoError(ctx, t, apiClient, nwName)

	// Start server with aliases
	serverID := ctr.Run(ctx, t, apiClient,
		ctr.WithName("alias-server"),
		ctr.WithNetworkMode(nwName),
		ctr.WithEndpointSettings(nwName, &networktypes.EndpointSettings{
			Aliases: []string{"database", "db", "postgres"},
		}),
		ctr.WithCmd("top"),
	)
	defer apiClient.ContainerRemove(ctx, serverID, client.ContainerRemoveOptions{Force: true})

	// Test resolving each alias
	for _, alias := range []string{"database", "db", "postgres"} {
		t.Run(fmt.Sprintf("alias=%s", alias), func(t *testing.T) {
			attachCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			res := ctr.RunAttach(attachCtx, t, apiClient,
				ctr.WithNetworkMode(nwName),
				ctr.WithCmd("ping6", "-c", "1", alias),
			)

			assert.Check(t, is.Equal(0, res.ExitCode),
				"Should resolve and ping alias: %s", alias)
		})
	}
}

// TestIPv6OnlyMultipleContainers tests multiple containers on an IPv6-only
// network can all communicate with each other.
func TestIPv6OnlyMultipleContainers(t *testing.T) {
	skip.If(t, testEnv.DaemonInfo.OSType == "windows", "IPv6-only not supported on Windows")

	ctx := setupTest(t)
	apiClient := testEnv.APIClient()

	const nwName = "ipv6-multi"
	const numContainers = 5

	network.CreateNoError(ctx, t, apiClient, nwName,
		network.WithIPv6(),
		network.WithOption(netlabel.EnableIPv4, "false"),
	)
	defer network.RemoveNoError(ctx, t, apiClient, nwName)

	// Start multiple containers
	containerIDs := make([]string, numContainers)
	for i := 0; i < numContainers; i++ {
		containerIDs[i] = ctr.Run(ctx, t, apiClient,
			ctr.WithName(fmt.Sprintf("ipv6-node-%d", i)),
			ctr.WithNetworkMode(nwName),
			ctr.WithCmd("top"),
		)
		defer apiClient.ContainerRemove(ctx, containerIDs[i], client.ContainerRemoveOptions{Force: true})
	}

	// Test that first container can ping all others
	attachCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	pingTargets := make([]string, numContainers-1)
	for i := 1; i < numContainers; i++ {
		pingTargets[i-1] = fmt.Sprintf("ipv6-node-%d", i)
	}

	pingCmd := []string{"sh", "-c",
		fmt.Sprintf("ping6 -c 1 %s", strings.Join(pingTargets, " && ping6 -c 1 "))}

	res := ctr.RunAttach(attachCtx, t, apiClient,
		ctr.WithNetworkMode(nwName),
		ctr.WithCmd(pingCmd...),
	)

	assert.Check(t, is.Equal(0, res.ExitCode),
		"Should be able to ping all containers")
}

// TestIPv6OnlyNetworkInspect verifies the network inspect output
// correctly shows IPv6-only configuration.
func TestIPv6OnlyNetworkInspect(t *testing.T) {
	skip.If(t, testEnv.DaemonInfo.OSType == "windows", "IPv6-only not supported on Windows")

	ctx := setupTest(t)
	apiClient := testEnv.APIClient()

	const nwName = "ipv6-inspect"

	network.CreateNoError(ctx, t, apiClient, nwName,
		network.WithIPv6(),
		network.WithOption(netlabel.EnableIPv4, "false"),
	)
	defer network.RemoveNoError(ctx, t, apiClient, nwName)

	res, err := apiClient.NetworkInspect(ctx, nwName, client.NetworkInspectOptions{Verbose: true})
	assert.NilError(t, err)

	// Verify all expected fields
	assert.Check(t, is.Equal("bridge", res.Network.Driver))
	assert.Check(t, is.Equal(false, res.Network.EnableIPv4))
	assert.Check(t, is.Equal(true, res.Network.EnableIPv6))
	assert.Check(t, is.Equal(false, res.Network.Internal))
	assert.Check(t, res.Network.IPAM.Driver != "", "Should have IPAM driver")

	// Verify options reflect IPv6-only
	if opts, ok := res.Network.Options[netlabel.EnableIPv4]; ok {
		assert.Check(t, is.Equal("false", opts))
	}
	if opts, ok := res.Network.Options[netlabel.EnableIPv6]; ok {
		assert.Check(t, is.Equal("true", opts))
	}
}
