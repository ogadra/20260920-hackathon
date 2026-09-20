// Package main は broker サービスのエントリポイントを提供する。
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ogadra/bunshin/broker/config"
	"github.com/ogadra/bunshin/broker/handler"
	"github.com/ogadra/bunshin/broker/healthcheck"
	"github.com/ogadra/bunshin/broker/service"
	"github.com/ogadra/bunshin/broker/store"
)

// newRouter は broker の HTTP ルーティングを構成した gin.Engine を返す。
// h が nil の場合はヘルスチェックのみ登録する。
func newRouter(h *handler.Handler) *gin.Engine {
	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "ok\n")
	})
	if h != nil {
		r.Use(handler.RequestIDMiddleware(handler.DefaultIDFn))
		r.DELETE("/sessions/:sessionId", h.DeleteSession)
		r.GET("/resolve/session", h.GetResolveSession)
		r.GET("/resolve/app", h.GetResolveApp)
		r.POST("/internal/runners/register", h.PostRegister)
		r.DELETE("/internal/runners/:runnerId", h.DeleteRunner)
		r.GET("/runners/busy", h.GetListBusyRunners)
	}
	return r
}

// stdout はメインの出力先。テスト時に差し替える。
var stdout io.Writer = os.Stdout

// addr はサーバーのリッスンアドレス。テスト時に差し替える。
var addr = ":8080"

// shutdownTimeout はグレースフルシャットダウンのタイムアウト。テスト時に差し替える。
var shutdownTimeout = 5 * time.Second

// fatalf はエラー時の終了処理。テスト時に差し替える。
var fatalf = log.Fatalf

// signalNotify は os/signal.Notify のラッパー。テスト時に差し替える。
var signalNotify = signal.Notify

// newBrokerService は BrokerService コンストラクタ。テスト時に差し替える。
var newBrokerService = func(repo store.Repository, stack string, checker healthcheck.Checker) service.Service {
	return service.NewBrokerService(repo, stack, service.WithChecker(checker))
}

// defaultInitHandler は環境変数を解決して Handler と後片付け用の Closer を組み立てる。
// 具体的な env 読解と client 生成は broker/config が担う。Repository が io.Closer を
// 実装する (Firestore の gRPC connection など) 場合は shutdown 時に閉じる責務を run に渡す。
func defaultInitHandler() (*handler.Handler, io.Closer, error) {
	stack, err := config.NewStackFromEnv()
	if err != nil {
		return nil, nil, err
	}
	runnerAPIPort, err := config.NewRunnerAPIPortFromEnv()
	if err != nil {
		return nil, nil, err
	}
	repo, err := config.NewRepositoryFromEnv(context.Background())
	if err != nil {
		return nil, nil, err
	}
	checker := healthcheck.NewHTTPChecker(&http.Client{Timeout: 3 * time.Second}, runnerAPIPort)
	svc := newBrokerService(repo, stack.Self, checker)
	return handler.NewHandler(svc, stack.Fallbacks, stack.Self), repoCloser(repo), nil
}

// repoCloser は Repository が io.Closer を実装しているならそのまま返し、
// 非対応の実装 (DynamoRepository) には noop を返す。
func repoCloser(repo store.Repository) io.Closer {
	if c, ok := repo.(io.Closer); ok {
		return c
	}
	return noopCloser{}
}

type noopCloser struct{}

func (noopCloser) Close() error { return nil }

// run はサーバーの起動とグレースフルシャットダウンを行う。
// initHandler は Handler と Repository の Close を構築する関数で、テスト時に fake を注入できる。
func run(initHandler func() (*handler.Handler, io.Closer, error)) error {
	h, closer, err := initHandler()
	if err != nil {
		return fmt.Errorf("init handler: %w", err)
	}
	defer func() {
		if err := closer.Close(); err != nil {
			fmt.Fprintf(stdout, "close repository: %v\n", err)
		}
	}()
	r := newRouter(h)

	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	fmt.Fprintf(stdout, "broker listening on %s\n", addr)

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signalNotify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case sig := <-quit:
		fmt.Fprintf(stdout, "received signal %s, shutting down...\n", sig)
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return srv.Shutdown(ctx)
}

// main は broker の HTTP サーバーを起動する。
func main() {
	if err := run(defaultInitHandler); err != nil {
		fatalf("server error: %v", err)
	}
}
