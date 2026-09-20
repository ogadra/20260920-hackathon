// Package healthcheck は runner の死活監視を提供する。
package healthcheck

import (
	"context"
	"fmt"
	"net/http"
)

// Checker は runner が到達可能かどうかをテストするインターフェース。
type Checker interface {
	// Check は指定された privateHost の runner に GET http://<privateHost>:<port>/health を実行する。
	// runner が正常であれば nil を、到達不能または異常であればエラーを返す。
	Check(ctx context.Context, privateHost string) error
}

// HTTPChecker は HTTP ベースの Checker 実装。
type HTTPChecker struct {
	client *http.Client
	port   int
}

// NewHTTPChecker は runner の管理 API port を固定した HTTPChecker を返す。
// client が nil、port が範囲外の場合は panic する (fallback を作らず起動時に落とす)。
func NewHTTPChecker(client *http.Client, port int) *HTTPChecker {
	if client == nil {
		panic("healthcheck: client must not be nil")
	}
	if port <= 0 || port > 65535 {
		panic("healthcheck: port must be between 1 and 65535")
	}
	return &HTTPChecker{client: client, port: port}
}

// Check は GET http://{privateHost}:{port}/health を実行し、200 であれば nil を返す。
// 200 以外のステータスコードまたはリクエストエラーの場合はエラーを返す。
func (c *HTTPChecker) Check(ctx context.Context, privateHost string) error {
	url := fmt.Sprintf("http://%s:%d/health", privateHost, c.port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("healthcheck: create request: %w", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("healthcheck: request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck: unexpected status %d", resp.StatusCode)
	}
	return nil
}
