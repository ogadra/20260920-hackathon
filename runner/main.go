package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// fatalf is the function called on fatal errors. It defaults to log.Fatalf
// and can be replaced in tests to avoid os.Exit.
var fatalf = log.Fatalf

// resolveIdentityFn is the function used to resolve the runner identity.
// It defaults to resolveIdentity and can be replaced in tests.
var resolveIdentityFn = resolveIdentity

// registerFn is the function used to register with the broker.
// It defaults to register and can be replaced in tests.
var registerFn = register

// deregisterFn is the function used to deregister from the broker on shutdown.
// It defaults to deregister and can be replaced in tests.
var deregisterFn = deregister

// main reads the RUNNER_API_PORT environment variable and starts the HTTP server
// with graceful shutdown on SIGTERM/SIGINT.
// The empty host binds to all interfaces, which is intentional for use inside a Docker container.
func main() {
	port := os.Getenv("RUNNER_API_PORT")
	if port == "" {
		fatalf("missing required environment variable: RUNNER_API_PORT")
		return
	}
	if err := start(":" + port); err != nil {
		fatalf("server error: %v", err)
	}
}

// start binds to the given address, resolves the runner identity,
// registers with the broker, and runs the server until a termination
// signal is received. It returns any error from the server lifecycle.
func start(addr string) error {
	// listenより先にSIGTERMの受け口を作る。
	// listen直後から接続は受け付くため、後からNotifyすると
	// その隙間に届いたSIGTERMが既定動作でプロセスを落とす。
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sig)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	identity, err := resolveIdentityFn(ctx, identityDeps{
		getenv:         os.Getenv,
		hostname:       os.Hostname,
		httpGet:        defaultHTTPGet,
		interfaceAddrs: net.InterfaceAddrs,
		randRead:       rand.Read,
	})
	if err != nil {
		ln.Close()
		return fmt.Errorf("resolve identity: %w", err)
	}
	log.Printf("runner identity: id=%s host=%s", identity.RunnerID, identity.PrivateHost)

	brokerURL := os.Getenv("BROKER_URL")
	if brokerURL == "" {
		ln.Close()
		return fmt.Errorf("missing required environment variable: BROKER_URL")
	}

	regCtx, regCancel := context.WithCancel(context.Background())
	go func() {
		select {
		case s := <-sig:
			regCancel()
			sig <- s
		case <-regCtx.Done():
		}
	}()
	err = registerFn(regCtx, registerDeps{
		brokerURL: brokerURL,
		identity:  identity,
		httpPost:  defaultHTTPPost,
		afterFunc: time.After,
		logf:      log.Printf,
	})
	regCancel()
	if err != nil {
		ln.Close()
		return fmt.Errorf("register with broker: %w", err)
	}

	cfg := serverConfig{
		sm:              NewShellManager(),
		shutdownTimeout: 10 * time.Second,
		brokerURL:       brokerURL,
		runnerID:        identity.RunnerID,
		superviseFn:     runAppSupervisorFn,
	}

	return run(ln, sig, cfg)
}

// serverConfig holds all dependencies needed to start and shut down the server.
// Fields default to production values when created via main.
type serverConfig struct {
	sm              *ShellManager
	shutdownTimeout time.Duration
	handler         http.Handler
	brokerURL       string
	runnerID        string
	superviseFn     func(ctx context.Context)
}

// run starts the HTTP server on the given listener and blocks until a signal is
// received on sigCh, then performs graceful shutdown.
// Separating this from main allows tests to inject a signal channel and listener.
func run(ln net.Listener, sigCh <-chan os.Signal, cfg serverConfig) error {
	h := cfg.handler
	if h == nil {
		h = newHandler(cfg.sm)
	}

	supCtx, supCancel := context.WithCancel(context.Background())
	supDone := make(chan struct{})
	go func() {
		defer close(supDone)
		cfg.superviseFn(supCtx)
	}()

	srv := &http.Server{
		Handler: h,
	}

	serveErr := make(chan error, 1)
	go func() {
		log.Printf("server listening on %s", ln.Addr())
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	select {
	case err := <-serveErr:
		supCancel()
		<-supDone
		return fmt.Errorf("serve: %w", err)
	case <-sigCh:
	}

	log.Println("shutting down...")

	supCancel()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.shutdownTimeout)
	defer cancel()

	if cfg.brokerURL != "" && cfg.runnerID != "" {
		if err := deregisterFn(ctx, deregisterDeps{
			brokerURL: cfg.brokerURL,
			runnerID:  cfg.runnerID,
			httpDo:    http.DefaultClient.Do,
			logf:      log.Printf,
		}); err != nil {
			log.Printf("WARNING: deregister from broker failed: %v", err)
		}
	}

	var firstErr error
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("server shutdown error: %v", err)
		firstErr = err
	}

	if err := cfg.sm.CloseAll(); err != nil {
		log.Printf("shell cleanup error: %v", err)
		if firstErr == nil {
			firstErr = err
		}
	}

	<-supDone

	log.Println("shutdown complete")
	return firstErr
}
