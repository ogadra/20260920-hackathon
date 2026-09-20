package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
)

// stackName values for privateHost resolution. There is intentionally no
// default: an unset or unrecognized STACK_NAME must fail startup rather than
// silently pick a resolution strategy.
const (
	stackAPNortheast1   = "ap-northeast-1"
	stackAPNortheast3   = "ap-northeast-3"
	stackAsiaNortheast1 = "asia-northeast1"
	stackAsiaNortheast2 = "asia-northeast2"
	stackLocal          = "local"
)

// Identity holds the runner registration parameters resolved at startup.
type Identity struct {
	// RunnerID is the unique identifier for this runner.
	RunnerID string
	// PrivateHost is the hostname (without port) that the broker uses to reach this runner.
	// port は broker と nginx がそれぞれ持つ用途別 port (RUNNER_API_PORT / RUNNER_APP_PORT) から貼る。
	PrivateHost string
}

// identityDeps holds injectable dependencies for identity resolution.
type identityDeps struct {
	getenv         func(string) string
	hostname       func() (string, error)
	httpGet        func(ctx context.Context, url string) ([]byte, error)
	interfaceAddrs func() ([]net.Addr, error)
	randRead       func([]byte) (int, error)
}

// ecsNetwork represents a single network attachment in ECS container metadata.
type ecsNetwork struct {
	// IPv4Addresses holds the IPv4 addresses assigned to the container.
	IPv4Addresses []string `json:"IPv4Addresses"`
}

// ecsContainerMeta is the subset of fields from GET $ECS_CONTAINER_METADATA_URI_V4.
type ecsContainerMeta struct {
	// Networks holds the network attachments for the container.
	Networks []ecsNetwork `json:"Networks"`
}

func resolveIdentity(ctx context.Context, deps identityDeps) (Identity, error) {
	runnerID, err := generateRunnerID(deps.randRead)
	if err != nil {
		return Identity{}, err
	}
	privateHost, err := resolvePrivateHost(ctx, deps)
	if err != nil {
		return Identity{}, err
	}
	return Identity{RunnerID: runnerID, PrivateHost: privateHost}, nil
}

func generateRunnerID(randRead func([]byte) (int, error)) (string, error) {
	var buf [16]byte
	// randRead follows the io.Reader convention where n < len(b) with nil err
	// is legal, so err alone is not enough to reject a partial read that would
	// silently leave the tail zero-filled.
	n, err := randRead(buf[:])
	if err != nil {
		return "", fmt.Errorf("generate runner id: %w", err)
	}
	if n != len(buf) {
		return "", fmt.Errorf("generate runner id: short read %d/%d", n, len(buf))
	}
	return hex.EncodeToString(buf[:]), nil
}

func resolvePrivateHost(ctx context.Context, deps identityDeps) (string, error) {
	stackName := deps.getenv("STACK_NAME")
	switch stackName {
	case stackAPNortheast1, stackAPNortheast3:
		return privateHostFromECS(ctx, deps)
	case stackAsiaNortheast1, stackAsiaNortheast2:
		return privateHostFromPodIP(deps)
	case stackLocal:
		return privateHostFromHostname(deps)
	default:
		return "", fmt.Errorf("unsupported STACK_NAME: %q", stackName)
	}
}

func privateHostFromECS(ctx context.Context, deps identityDeps) (string, error) {
	ecsURI := deps.getenv("ECS_CONTAINER_METADATA_URI_V4")
	if ecsURI == "" {
		return "", fmt.Errorf("missing required environment variable: ECS_CONTAINER_METADATA_URI_V4")
	}
	body, err := deps.httpGet(ctx, ecsURI)
	if err != nil {
		return "", fmt.Errorf("fetch ECS container metadata: %w", err)
	}
	var container ecsContainerMeta
	if err := json.Unmarshal(body, &container); err != nil {
		return "", fmt.Errorf("parse ECS container metadata: %w", err)
	}
	if len(container.Networks) == 0 || len(container.Networks[0].IPv4Addresses) == 0 {
		return "", fmt.Errorf("no IPv4 address in ECS container metadata")
	}
	return container.Networks[0].IPv4Addresses[0], nil
}

// privateHostFromPodIP resolves the runner's own address on GKE, where there is
// no metadata endpoint analogous to ECS: the Pod's IP is only observable from
// inside the container via its network interfaces.
func privateHostFromPodIP(deps identityDeps) (string, error) {
	addrs, err := deps.interfaceAddrs()
	if err != nil {
		return "", fmt.Errorf("get interface addresses: %w", err)
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() {
			continue
		}
		if ip4 := ipNet.IP.To4(); ip4 != nil {
			return ip4.String(), nil
		}
	}
	return "", fmt.Errorf("no non-loopback IPv4 address found")
}

func privateHostFromHostname(deps identityDeps) (string, error) {
	host, err := deps.hostname()
	if err != nil {
		return "", fmt.Errorf("get hostname: %w", err)
	}
	return host, nil
}

// defaultHTTPGet performs an HTTP GET request and returns the response body.
// It is used for fetching ECS metadata from the local endpoint.
func defaultHTTPGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metadata endpoint returned %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
