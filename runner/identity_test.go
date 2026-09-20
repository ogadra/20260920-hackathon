package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func stubRandRead(fill byte, err error) func([]byte) (int, error) {
	return func(b []byte) (int, error) {
		if err != nil {
			return 0, err
		}
		for i := range b {
			b[i] = fill
		}
		return len(b), nil
	}
}

// stubGetenv returns a getenv func backed by the given key/value map, mirroring
// os.Getenv's behavior of returning "" for unset keys.
func stubGetenv(values map[string]string) func(string) string {
	return func(k string) string { return values[k] }
}

func TestResolveIdentityECS(t *testing.T) {
	containerJSON := `{"Networks":[{"IPv4Addresses":["10.0.1.5"]}]}`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(containerJSON))
	}))
	defer ts.Close()

	deps := identityDeps{
		getenv: stubGetenv(map[string]string{
			"STACK_NAME":                    "ap-northeast-1",
			"ECS_CONTAINER_METADATA_URI_V4": ts.URL,
		}),
		hostname: func() (string, error) { return "", errors.New("should not be called") },
		httpGet:  defaultHTTPGet,
		randRead: stubRandRead(0xab, nil),
	}

	id, err := resolveIdentity(context.Background(), deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "abababababababababababababababab"
	if id.RunnerID != want {
		t.Errorf("RunnerID = %q, want %q", id.RunnerID, want)
	}
	if id.PrivateHost != "10.0.1.5" {
		t.Errorf("PrivateHost = %q, want %q", id.PrivateHost, "10.0.1.5")
	}
}

func TestResolveIdentityECSApNortheast3(t *testing.T) {
	containerJSON := `{"Networks":[{"IPv4Addresses":["10.0.1.6"]}]}`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(containerJSON))
	}))
	defer ts.Close()

	deps := identityDeps{
		getenv: stubGetenv(map[string]string{
			"STACK_NAME":                    "ap-northeast-3",
			"ECS_CONTAINER_METADATA_URI_V4": ts.URL,
		}),
		httpGet:  defaultHTTPGet,
		randRead: stubRandRead(0x01, nil),
	}

	id, err := resolveIdentity(context.Background(), deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.PrivateHost != "10.0.1.6" {
		t.Errorf("PrivateHost = %q, want %q", id.PrivateHost, "10.0.1.6")
	}
}

func TestResolveIdentityECSMissingMetadataURI(t *testing.T) {
	deps := identityDeps{
		getenv:   stubGetenv(map[string]string{"STACK_NAME": "ap-northeast-1"}),
		randRead: stubRandRead(0x01, nil),
	}

	_, err := resolveIdentity(context.Background(), deps)
	if err == nil {
		t.Fatal("expected error when ECS_CONTAINER_METADATA_URI_V4 is missing")
	}
}

func TestResolveIdentityRandReadError(t *testing.T) {
	deps := identityDeps{
		getenv:   stubGetenv(map[string]string{"STACK_NAME": "local"}),
		hostname: func() (string, error) { return "host", nil },
		randRead: stubRandRead(0, errors.New("no entropy")),
	}

	_, err := resolveIdentity(context.Background(), deps)
	if err == nil {
		t.Fatal("expected error when randRead fails")
	}
}

func TestResolveIdentityRandShortRead(t *testing.T) {
	deps := identityDeps{
		getenv:   stubGetenv(map[string]string{"STACK_NAME": "local"}),
		hostname: func() (string, error) { return "host", nil },
		randRead: func(b []byte) (int, error) {
			if len(b) == 0 {
				return 0, nil
			}
			b[0] = 0xff
			return 1, nil
		},
	}

	_, err := resolveIdentity(context.Background(), deps)
	if err == nil {
		t.Fatal("expected error when randRead returns a short read")
	}
}

func TestResolveIdentityECSContainerFetchError(t *testing.T) {
	deps := identityDeps{
		getenv: stubGetenv(map[string]string{
			"STACK_NAME":                    "ap-northeast-1",
			"ECS_CONTAINER_METADATA_URI_V4": "http://169.254.170.2/v4",
		}),
		httpGet: func(ctx context.Context, url string) ([]byte, error) {
			return nil, errors.New("connection refused")
		},
		randRead: stubRandRead(0x01, nil),
	}

	_, err := resolveIdentity(context.Background(), deps)
	if err == nil {
		t.Fatal("expected error when container metadata fetch fails")
	}
}

func TestResolveIdentityECSContainerInvalidJSON(t *testing.T) {
	deps := identityDeps{
		getenv: stubGetenv(map[string]string{
			"STACK_NAME":                    "ap-northeast-1",
			"ECS_CONTAINER_METADATA_URI_V4": "http://169.254.170.2/v4",
		}),
		httpGet: func(ctx context.Context, url string) ([]byte, error) {
			return []byte("{bad"), nil
		},
		randRead: stubRandRead(0x01, nil),
	}

	_, err := resolveIdentity(context.Background(), deps)
	if err == nil {
		t.Fatal("expected error when container metadata is invalid JSON")
	}
}

func TestResolveIdentityECSEmptyNetworks(t *testing.T) {
	deps := identityDeps{
		getenv: stubGetenv(map[string]string{
			"STACK_NAME":                    "ap-northeast-1",
			"ECS_CONTAINER_METADATA_URI_V4": "http://169.254.170.2/v4",
		}),
		httpGet: func(ctx context.Context, url string) ([]byte, error) {
			return []byte(`{"Networks":[]}`), nil
		},
		randRead: stubRandRead(0x01, nil),
	}

	_, err := resolveIdentity(context.Background(), deps)
	if err == nil {
		t.Fatal("expected error when Networks is empty")
	}
}

func TestResolveIdentityECSEmptyIPv4(t *testing.T) {
	deps := identityDeps{
		getenv: stubGetenv(map[string]string{
			"STACK_NAME":                    "ap-northeast-1",
			"ECS_CONTAINER_METADATA_URI_V4": "http://169.254.170.2/v4",
		}),
		httpGet: func(ctx context.Context, url string) ([]byte, error) {
			return []byte(`{"Networks":[{"IPv4Addresses":[]}]}`), nil
		},
		randRead: stubRandRead(0x01, nil),
	}

	_, err := resolveIdentity(context.Background(), deps)
	if err == nil {
		t.Fatal("expected error when IPv4Addresses is empty")
	}
}

func TestResolveIdentityGKEPodIP(t *testing.T) {
	deps := identityDeps{
		getenv: stubGetenv(map[string]string{"STACK_NAME": "asia-northeast1"}),
		interfaceAddrs: func() ([]net.Addr, error) {
			return []net.Addr{
				&net.IPNet{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(8, 32)},
				&net.IPNet{IP: net.ParseIP("10.4.0.9"), Mask: net.CIDRMask(24, 32)},
			}, nil
		},
		randRead: stubRandRead(0x02, nil),
	}

	id, err := resolveIdentity(context.Background(), deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.PrivateHost != "10.4.0.9" {
		t.Errorf("PrivateHost = %q, want %q", id.PrivateHost, "10.4.0.9")
	}
}

func TestResolveIdentityGKEAsiaNortheast2(t *testing.T) {
	deps := identityDeps{
		getenv: stubGetenv(map[string]string{"STACK_NAME": "asia-northeast2"}),
		interfaceAddrs: func() ([]net.Addr, error) {
			return []net.Addr{
				&net.IPNet{IP: net.ParseIP("10.4.0.10"), Mask: net.CIDRMask(24, 32)},
			}, nil
		},
		randRead: stubRandRead(0x02, nil),
	}

	id, err := resolveIdentity(context.Background(), deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.PrivateHost != "10.4.0.10" {
		t.Errorf("PrivateHost = %q, want %q", id.PrivateHost, "10.4.0.10")
	}
}

func TestResolveIdentityGKEInterfaceAddrsError(t *testing.T) {
	deps := identityDeps{
		getenv: stubGetenv(map[string]string{"STACK_NAME": "asia-northeast1"}),
		interfaceAddrs: func() ([]net.Addr, error) {
			return nil, errors.New("interface lookup failed")
		},
		randRead: stubRandRead(0x02, nil),
	}

	_, err := resolveIdentity(context.Background(), deps)
	if err == nil {
		t.Fatal("expected error when interfaceAddrs fails")
	}
}

func TestResolveIdentityGKENoIPv4(t *testing.T) {
	deps := identityDeps{
		getenv: stubGetenv(map[string]string{"STACK_NAME": "asia-northeast1"}),
		interfaceAddrs: func() ([]net.Addr, error) {
			return []net.Addr{
				&net.IPNet{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(8, 32)},
				&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
			}, nil
		},
		randRead: stubRandRead(0x02, nil),
	}

	_, err := resolveIdentity(context.Background(), deps)
	if err == nil {
		t.Fatal("expected error when no non-loopback IPv4 address is found")
	}
}

func TestResolveIdentityLocalHostname(t *testing.T) {
	deps := identityDeps{
		getenv:   stubGetenv(map[string]string{"STACK_NAME": "local"}),
		hostname: func() (string, error) { return "runner-host", nil },
		httpGet: func(ctx context.Context, url string) ([]byte, error) {
			return nil, errors.New("should not be called")
		},
		randRead: stubRandRead(0xcd, nil),
	}

	id, err := resolveIdentity(context.Background(), deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantID := "cdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd"
	if id.RunnerID != wantID {
		t.Errorf("RunnerID = %q, want %q", id.RunnerID, wantID)
	}
	if id.PrivateHost != "runner-host" {
		t.Errorf("PrivateHost = %q, want %q", id.PrivateHost, "runner-host")
	}
}

func TestResolveIdentityLocalHostnameError(t *testing.T) {
	deps := identityDeps{
		getenv:   stubGetenv(map[string]string{"STACK_NAME": "local"}),
		hostname: func() (string, error) { return "", errors.New("no hostname") },
		randRead: stubRandRead(0x01, nil),
	}

	_, err := resolveIdentity(context.Background(), deps)
	if err == nil {
		t.Fatal("expected error when hostname fails")
	}
}

func TestResolveIdentityStackNameMissing(t *testing.T) {
	deps := identityDeps{
		getenv:   stubGetenv(nil),
		randRead: stubRandRead(0x01, nil),
	}

	_, err := resolveIdentity(context.Background(), deps)
	if err == nil {
		t.Fatal("expected error when STACK_NAME is missing")
	}
}

func TestResolveIdentityStackNameUnsupported(t *testing.T) {
	deps := identityDeps{
		getenv:   stubGetenv(map[string]string{"STACK_NAME": "us-east-1"}),
		randRead: stubRandRead(0x01, nil),
	}

	_, err := resolveIdentity(context.Background(), deps)
	if err == nil {
		t.Fatal("expected error when STACK_NAME is unsupported")
	}
}

func TestDefaultHTTPGetSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	body, err := defaultHTTPGet(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", string(body), "ok")
	}
}

func TestDefaultHTTPGetNon200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	_, err := defaultHTTPGet(context.Background(), ts.URL)
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
}

func TestDefaultHTTPGetCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := defaultHTTPGet(ctx, "http://127.0.0.1:1/unreachable")
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

func TestDefaultHTTPGetInvalidURL(t *testing.T) {
	_, err := defaultHTTPGet(context.Background(), "://bad-url")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}
