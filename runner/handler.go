package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
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

// errMissingShellCookie is the error message returned when the shell_id cookie is absent.
const errMissingShellCookie = "missing shell_id cookie"

const errMissingClientAddressHeader = "missing X-Bunshin-Client-Address header"

const errInvalidClientAddressHeader = "invalid X-Bunshin-Client-Address header"

// Audit-log classes for the outcome of validating a command.
const (
	classAccepted              = "accepted"
	classRejected              = "rejected"
	classValidationError       = "validation-error"
	classValidationUnavailable = "validation-unavailable"
)

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
func newHandler(sm *ShellManager, v Validator) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.SetTrustedProxies([]string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"})
	r.HandleMethodNotAllowed = true
	r.GET("/health", handleHealth())
	r.POST("/api/shell", handleCreateShell(sm))
	r.DELETE("/api/shell", handleDeleteShell(sm))
	r.POST("/api/execute", handleExecute(sm, v))
	return r
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
// Every command is submitted to the validator before it reaches the shell.
func handleExecute(sm *ShellManager, v Validator) gin.HandlerFunc {
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

		remote, err := clientAddress(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		result, err := v.Validate(c.Request.Context(), req.Command)
		if err != nil {
			var unavailable *ValidationUnavailableError
			if errors.As(err, &unavailable) {
				auditLog(id, remote, classValidationUnavailable, req.Command, nil, err)
				c.JSON(http.StatusServiceUnavailable, errorResponse{Error: err.Error()})
				return
			}
			auditLog(id, remote, classValidationError, req.Command, nil, err)
			c.JSON(http.StatusForbidden, errorResponse{Error: err.Error()})
			return
		}
		if !result.Safe {
			auditLog(id, remote, classRejected, req.Command, nil, nil)
			c.JSON(http.StatusForbidden, errorResponse{Error: result.Reason})
			return
		}
		auditLog(id, remote, classAccepted, req.Command, nil, nil)

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

		auditLog(id, remote, classAccepted, req.Command, &exitCode, execErr)
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
