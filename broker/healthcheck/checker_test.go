// Package healthcheck は runner の死活監視のテストを提供する。
package healthcheck

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// splitHostPort は httptest.NewServer の URL から host と port を分離する。
func splitHostPort(t *testing.T, u string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(u, "http://"))
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return host, port
}

// TestHTTPChecker_ImplementsChecker は HTTPChecker が Checker インターフェースを満たすことを検証する。
func TestHTTPChecker_ImplementsChecker(t *testing.T) {
	t.Parallel()
	var _ Checker = (*HTTPChecker)(nil)
}

// TestNewHTTPChecker はコンストラクタが渡した client と port を保持することを検証する。
func TestNewHTTPChecker(t *testing.T) {
	t.Parallel()
	c := NewHTTPChecker(http.DefaultClient, 3000)
	if c == nil {
		t.Fatal("expected non-nil checker")
	}
	if c.client != http.DefaultClient {
		t.Error("client mismatch")
	}
	if c.port != 3000 {
		t.Errorf("port = %d, want 3000", c.port)
	}
}

// TestNewHTTPChecker_NilClient は client=nil を渡すと panic することを検証する (fallback しない)。
func TestNewHTTPChecker_NilClient(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil client")
		}
	}()
	NewHTTPChecker(nil, 3000)
}

// TestNewHTTPChecker_InvalidPort は range 外の port が panic することを検証する。
func TestNewHTTPChecker_InvalidPort(t *testing.T) {
	t.Parallel()
	for _, port := range []int{-1, 0, 65536, 100000} {
		port := port
		t.Run(strconv.Itoa(port), func(t *testing.T) {
			t.Parallel()
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("expected panic for port %d", port)
				}
			}()
			NewHTTPChecker(http.DefaultClient, port)
		})
	}
}

// TestHTTPChecker_Check_Healthy は runner が 200 を返す場合に nil を返すことを検証する。
func TestHTTPChecker_Check_Healthy(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/health")
		}
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want %q", r.Method, http.MethodGet)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	host, port := splitHostPort(t, srv.URL)
	checker := NewHTTPChecker(srv.Client(), port)
	err := checker.Check(context.Background(), host)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHTTPChecker_Check_Unhealthy は runner が 500 を返す場合にエラーを返すことを検証する。
func TestHTTPChecker_Check_Unhealthy(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	host, port := splitHostPort(t, srv.URL)
	checker := NewHTTPChecker(srv.Client(), port)
	err := checker.Check(context.Background(), host)
	if err == nil {
		t.Fatal("expected error for unhealthy runner")
	}
	if !strings.Contains(err.Error(), "unexpected status 500") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "unexpected status 500")
	}
}

// TestHTTPChecker_Check_Unreachable は listening していない port に接続できない場合にエラーを返すことを検証する。
func TestHTTPChecker_Check_Unreachable(t *testing.T) {
	t.Parallel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	host, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	_ = ln.Close()
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	client := &http.Client{Timeout: 1 * time.Second}
	checker := NewHTTPChecker(client, port)
	err = checker.Check(context.Background(), host)
	if err == nil {
		t.Fatal("expected error for unreachable runner")
	}
	if !strings.Contains(err.Error(), "request failed") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "request failed")
	}
}

// TestHTTPChecker_Check_InvalidHost は http.NewRequest が URL parse で fail する host を受けたときにエラーを返すことを検証する。
// register 側は validateRunnerHost で拒否されるが、Store が破損した場合の防御として。
func TestHTTPChecker_Check_InvalidHost(t *testing.T) {
	t.Parallel()
	checker := NewHTTPChecker(http.DefaultClient, 3000)
	err := checker.Check(context.Background(), "runner%zz")
	if err == nil {
		t.Fatal("expected error for invalid host")
	}
	if !strings.Contains(err.Error(), "create request") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "create request")
	}
}

// TestHTTPChecker_Check_ContextCanceled はコンテキストキャンセル時にエラーを返すことを検証する。
func TestHTTPChecker_Check_ContextCanceled(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	host, port := splitHostPort(t, srv.URL)
	checker := NewHTTPChecker(srv.Client(), port)
	err := checker.Check(ctx, host)
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}
