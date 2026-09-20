package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// Shell abstracts command execution and lifecycle for handler and shell manager.
// In production, *bashShell implements this interface.
type Shell interface {
	ExecuteStream(ctx context.Context, command string, stdoutCh chan<- string) (int, string, error)
	Close() error
}

// shellIDCookie is the cookie name used to pass the shell ID.
const shellIDCookie = "shell_id"

const clientAddressHeader = "X-Bunshin-Client-Address"

// handlerUpdateClass is the audit-log class recorded for PUT /api/app/handler
// so handler edits show up alongside /api/execute in the log stream.
const handlerUpdateClass = "handler-update"

// handlerAppFilePath is the perl demo handler that server.pl re-loads per
// request. Kept as a var so tests can point it at a temp file.
var handlerAppFilePath = "/app/DaiKichijoji.pm"

// handlerAppMaxSize caps PUT /api/app/handler body length; the demo handler is
// a page or two of Perl, so 1 MiB is well above any legitimate edit. Kept as
// a var so tests can lower it without allocating megabyte bodies.
var handlerAppMaxSize int64 = 1 << 20

// errMissingShellCookie is the error message returned when the shell_id cookie is absent.
const errMissingShellCookie = "missing shell_id cookie"

const errMissingClientAddressHeader = "missing X-Bunshin-Client-Address header"

const errInvalidClientAddressHeader = "invalid X-Bunshin-Client-Address header"

// executeRequest is the JSON body for POST /api/execute.
type executeRequest struct {
	Command string `json:"command" binding:"required"`
}

// errorResponse is the JSON body returned for error responses.
type errorResponse struct {
	Error string `json:"error"`
}

// sseEvent represents a single Server-Sent Event sent during command execution.
type sseEvent struct {
	Type     string `json:"type"`
	Data     string `json:"data,omitempty"`
	ExitCode *int   `json:"exitCode,omitempty"`
}

// newHandler creates a gin.Engine with all API routes registered.
// The returned engine handles GET /health, POST /api/shell, DELETE /api/shell, and POST /api/execute.
func newHandler(sm *ShellManager) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.SetTrustedProxies([]string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"})
	r.HandleMethodNotAllowed = true
	r.GET("/health", handleHealth())
	r.POST("/api/shell", handleCreateShell(sm))
	r.DELETE("/api/shell", handleDeleteShell(sm))
	r.POST("/api/execute", handleExecute(sm))
	r.GET("/api/app/handler", handleGetAppHandler())
	r.PUT("/api/app/handler", handlePutAppHandler())
	return r
}

// handleGetAppHandler returns a gin handler for GET /api/app/handler.
// It reads /app/DaiKichijoji.pm and returns the current contents so the front
// editor can seed itself without shell state.
func handleGetAppHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := os.ReadFile(handlerAppFilePath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		c.Data(http.StatusOK, "text/plain; charset=utf-8", data)
	}
}

// handlePutAppHandler returns a gin handler for PUT /api/app/handler.
// It writes the raw request body to /app/DaiKichijoji.pm atomically (tmp + rename)
// so server.pl never `do`'s a half-written file. Requires
// X-Bunshin-Client-Address and records an audit-log line, same shape as
// /api/execute.
func handlePutAppHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		remote, err := clientAddress(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, handlerAppMaxSize)
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: fmt.Sprintf("read body: %s", err.Error())})
			return
		}
		if err := writeHandlerAtomically(handlerAppFilePath, body); err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		auditLog("", remote, handlerUpdateClass, string(body), nil, nil)
		c.Status(http.StatusNoContent)
	}
}

// writeHandlerTmpCounterは並行PUTがそれぞれ独立したtmp pathを持てるよう単調に増える。
// PIDと組み合わせて十分ユニークになり、fixed nameによるwriter競合を防ぐ。
var writeHandlerTmpCounter atomic.Uint64

// writeHandlerAtomically writes data to a unique sibling temp file and renames
// it over the target so a concurrent server.pl re-load never observes a partial
// file. Rename is atomic on POSIX. gin per-request goroutinesは同じhandlerを並行実行するため
// 固定tmp名では書き込みが交錯して破損したファイルがrenameで確定される。counter + PIDでunique化する。
func writeHandlerAtomically(path string, data []byte) error {
	dir, base := filepath.Dir(path), filepath.Base(path)
	tmp := filepath.Join(dir, fmt.Sprintf(".%s.%d.%d.tmp", base, os.Getpid(), writeHandlerTmpCounter.Add(1)))
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// handleHealth returns a gin handler for GET /health.
// It returns 200 OK with body "ok\n" to indicate the runner is reachable.
func handleHealth() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.String(http.StatusOK, "ok\n")
	}
}

// handleCreateShell returns a gin handler for POST /api/shell.
// It creates a new shell and sets the shell_id cookie.
func handleCreateShell(sm *ShellManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _, err := sm.Create()
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		c.SetSameSite(http.SameSiteStrictMode)
		c.SetCookie(shellIDCookie, id, 0, "/", "", true, true)
		c.Status(http.StatusNoContent)
	}
}

// handleDeleteShell returns a gin handler for DELETE /api/shell.
// It deletes the shell specified by shell_id cookie.
func handleDeleteShell(sm *ShellManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := c.Cookie(shellIDCookie)
		if err != nil || id == "" {
			c.JSON(http.StatusBadRequest, errorResponse{Error: errMissingShellCookie})
			return
		}
		if err := sm.Delete(id); err != nil {
			if errors.Is(err, ErrShellNotFound) {
				c.JSON(http.StatusNotFound, errorResponse{Error: err.Error()})
			} else {
				c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
			}
			return
		}
		c.SetSameSite(http.SameSiteStrictMode)
		c.SetCookie(shellIDCookie, "", -1, "/", "", true, true)
		c.Status(http.StatusNoContent)
	}
}

// handleExecute returns a gin handler for POST /api/execute.
// It classifies the command for audit logging and executes it in the shell.
func handleExecute(sm *ShellManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := c.Cookie(shellIDCookie)
		if err != nil || id == "" {
			c.JSON(http.StatusBadRequest, errorResponse{Error: errMissingShellCookie})
			return
		}

		// Get only returns ErrShellNotFound; no other error paths exist.
		shell, err := sm.Get(id)
		if err != nil {
			c.JSON(http.StatusNotFound, errorResponse{Error: err.Error()})
			return
		}

		var req executeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: fmt.Sprintf("invalid request: %s", err.Error())})
			return
		}

		class := classifyCommand(req.Command)
		remote, err := clientAddress(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		auditLog(id, remote, class, req.Command, nil, nil)

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")

		ch := make(chan string, 100)
		done := make(chan struct{})
		go func() {
			defer close(done)
			for line := range ch {
				writeSSE(c.Writer, sseEvent{Type: "stdout", Data: line})
				c.Writer.Flush()
			}
		}()

		exitCode, stderr, execErr := shell.ExecuteStream(c.Request.Context(), req.Command, ch)
		<-done

		if stderr != "" {
			writeSSE(c.Writer, sseEvent{Type: "stderr", Data: stderr})
			c.Writer.Flush()
		}

		writeSSE(c.Writer, sseEvent{Type: "complete", ExitCode: &exitCode})
		c.Writer.Flush()

		auditLog(id, remote, class, req.Command, &exitCode, execErr)
	}
}

// auditLog writes a structured audit log line.
// exitCode and err are optional and only appended when non-nil.
func auditLog(shell, remote, class, command string, exitCode *int, err error) {
	msg := fmt.Sprintf("%s [AUDIT] shell=%s remote=%s class=%s command=%q", time.Now().Format("2006/01/02 15:04:05.000"), shell, remote, class, command)
	if exitCode != nil {
		msg += fmt.Sprintf(" exitCode=%d", *exitCode)
	}
	if err != nil {
		msg += fmt.Sprintf(" error=%v", err)
	}
	fmt.Fprintln(log.Writer(), msg)
}

// writeSSE marshals an sseEvent to JSON and writes it as a Server-Sent Event line.
// sseEvent contains only string and *int fields, so json.Marshal cannot fail.
func writeSSE(w http.ResponseWriter, event sseEvent) {
	data, _ := json.Marshal(event)
	fmt.Fprintf(w, "data: %s\n\n", data)
}

func clientAddress(c *gin.Context) (string, error) {
	value := c.GetHeader(clientAddressHeader)
	if value == "" {
		return "", errors.New(errMissingClientAddressHeader)
	}
	return parseClientAddress(value)
}

func parseClientAddress(value string) (string, error) {
	addr, err := netip.ParseAddrPort(value)
	if err == nil && addr.Port() != 0 {
		return addr.String(), nil
	}

	portStart := strings.LastIndexByte(value, ':')
	if portStart <= 0 || portStart == len(value)-1 {
		return "", errors.New(errInvalidClientAddressHeader)
	}
	ip, ipErr := netip.ParseAddr(value[:portStart])
	port, portErr := strconv.ParseUint(value[portStart+1:], 10, 16)
	if ipErr != nil || portErr != nil || port == 0 {
		return "", errors.New(errInvalidClientAddressHeader)
	}
	return netip.AddrPortFrom(ip, uint16(port)).String(), nil
}
