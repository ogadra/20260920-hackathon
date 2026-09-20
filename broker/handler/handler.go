// Package handler は broker の HTTP ハンドラーを提供する。
package handler

import (
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ogadra/bunshin/broker/model"
	"github.com/ogadra/bunshin/broker/service"
	"github.com/ogadra/bunshin/broker/stacklist"
	"github.com/ogadra/bunshin/broker/store"
)

var runnerHostRe = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)

var sessionHexRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

// sessionIDCookie は session 識別用の cookie 名。
const sessionIDCookie = "session_id"

const fallbackStackHeader = "X-Fallback-Stack"

const fallbackRemainingHeader = "X-Fallback-Remaining"

const runnerHostHeader = "X-Runner-Host"

const reassignedHeader = "X-Session-Reassigned"

// sessionHexHeaderはfrontがpreview URLを組むためのsession hexを返すヘッダー名。
// cookieはHttpOnlyのため、JSから読める経路としてヘッダーで返す。
const sessionHexHeader = "X-Session-Hex"

// stackNameHeaderはbroker自身のstack名をfrontに伝えるヘッダー名。
// front / composeのinterpolationで上書きされないようbrokerをsingle sourceとする。
const stackNameHeader = "X-Stack-Name"

// Handler は broker の HTTP ハンドラー。
type Handler struct {
	svc            service.Service
	fallbackStacks []string
	stackSelf      string
}

// NewHandlerはHandlerを生成する。svcがnil、stackSelfが空文字の場合はpanicする。
func NewHandler(svc service.Service, fallbackStacks []string, stackSelf string) *Handler {
	if svc == nil {
		panic("handler: nil service")
	}
	if stackSelf == "" {
		panic("handler: empty stackSelf")
	}
	return &Handler{svc: svc, fallbackStacks: fallbackStacks, stackSelf: stackSelf}
}

// registerRequest は POST /internal/runners/register のリクエストボディ。
type registerRequest struct {
	// RunnerID は runner の一意識別子。
	RunnerID string `json:"runnerId" binding:"required"`
	// PrivateHost は runner の hostname (port を含まない)。
	// 用途別 port (RUNNER_API_PORT / RUNNER_APP_PORT) は broker と nginx がそれぞれ知る。
	PrivateHost string `json:"privateHost" binding:"required"`
}

// DeleteSession は DELETE /sessions/:sessionId を処理しセッションを終了する。
func (h *Handler) DeleteSession(c *gin.Context) {
	sessionID := c.Param("sessionId")
	err := h.svc.CloseSession(c.Request.Context(), sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(c, http.StatusNotFound, model.CodeSessionNotFound, "session not found")
			return
		}
		writeError(c, http.StatusInternalServerError, model.CodeInternalError, "failed to close session")
		return
	}
	c.Status(http.StatusNoContent)
}

// GetResolveSession は GET /resolve/session を処理し session_id cookie からセッションを解決する。
// cookie が無い、またはセッションが見つからない場合は新規作成して Set-Cookie を返す。
func (h *Handler) GetResolveSession(c *gin.Context) {
	sessionID, _ := c.Cookie(sessionIDCookie)
	result, err := h.svc.ResolveSession(c.Request.Context(), sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNoIdleRunner) {
			h.signalFallback(c)
			writeError(c, http.StatusServiceUnavailable, model.CodeNoIdleRunner, "no idle runner available")
			return
		}
		writeError(c, http.StatusInternalServerError, model.CodeInternalError, "failed to resolve session")
		return
	}
	if result.Created {
		c.SetSameSite(http.SameSiteStrictMode)
		c.SetCookie(sessionIDCookie, result.SessionID, 0, "/", "", true, true)
	}
	if result.Reassigned {
		c.Header(reassignedHeader, "true")
	}
	c.Header(sessionHexHeader, result.SessionHex)
	c.Header(stackNameHeader, h.stackSelf)
	c.Header(runnerHostHeader, result.RunnerHost)
	c.Status(http.StatusOK)
}

// GetResolveApp は GET /resolve/app を処理し Host の先頭 hex ラベルから所属 runner を引く。
// port-forward 用に session 割り当ては行わず、既存 session の runner host のみ返す。
// 自 stack / internal_domain 完全一致は nginx で完結しているため、broker は hex ラベルだけ検証する。
func (h *Handler) GetResolveApp(c *gin.Context) {
	hex, ok := extractSessionHex(c.Request.Host)
	if !ok {
		writeError(c, http.StatusBadRequest, model.CodeInvalidRequest, "Host must start with 32 lowercase hex characters followed by a dot")
		return
	}
	result, err := h.svc.LookupSession(c.Request.Context(), hex)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(c, http.StatusNotFound, model.CodeSessionNotFound, "session not found")
			return
		}
		writeError(c, http.StatusInternalServerError, model.CodeInternalError, "failed to look up session")
		return
	}
	c.Header(runnerHostHeader, result.RunnerHost)
	c.Status(http.StatusOK)
}

func extractSessionHex(host string) (string, bool) {
	label, _, ok := strings.Cut(host, ".")
	if !ok {
		return "", false
	}
	if !sessionHexRe.MatchString(label) {
		return "", false
	}
	return label, true
}

// X-Fallback-Stack の有無を転送済み判定に兼用し、専用のマーカーヘッダを増やさない。
func (h *Handler) signalFallback(c *gin.Context) {
	pool := h.fallbackStacks
	if c.GetHeader(fallbackStackHeader) != "" {
		pool = stacklist.Split(c.GetHeader(fallbackRemainingHeader))
	}
	if len(pool) == 0 {
		log.Printf("fallback_signal unavailable request_id=%s session_id=%s", requestID(c), sessionCookie(c))
		return
	}
	c.Header(fallbackStackHeader, pool[0])
	if len(pool) > 1 {
		c.Header(fallbackRemainingHeader, strings.Join(pool[1:], ","))
	}
	log.Printf(
		"fallback_signal next_stack=%s remaining=%s request_id=%s session_id=%s",
		pool[0],
		strings.Join(pool[1:], ","),
		requestID(c),
		sessionCookie(c),
	)
}

func requestID(c *gin.Context) string {
	reqID, _ := c.Get(requestIDKey)
	rid, _ := reqID.(string)
	return rid
}

func sessionCookie(c *gin.Context) string {
	sessionID, _ := c.Cookie(sessionIDCookie)
	return sessionID
}

// validateRunnerHost は register 時に受けた host label を検証する。
// 用途別 port は broker / nginx がそれぞれ env で持つため、ここで port を検証しない。
func validateRunnerHost(host string) error {
	if host == "" {
		return errors.New("host is required")
	}
	if !runnerHostRe.MatchString(host) {
		return errors.New("host must be hostname-style")
	}
	return nil
}

// PostRegister は POST /internal/runners/register を処理し runner を登録する。
func (h *Handler) PostRegister(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, model.CodeInvalidRequest, "invalid request body")
		return
	}
	if err := validateRunnerHost(req.PrivateHost); err != nil {
		writeError(c, http.StatusBadRequest, model.CodeInvalidRequest, "invalid privateHost: "+err.Error())
		return
	}
	err := h.svc.RegisterRunner(c.Request.Context(), req.RunnerID, req.PrivateHost)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(c, http.StatusConflict, model.CodeRunnerConflict, "runner already registered with different attributes")
			return
		}
		writeError(c, http.StatusInternalServerError, model.CodeInternalError, "failed to register runner")
		return
	}
	c.Status(http.StatusCreated)
}

// DeleteRunner は DELETE /internal/runners/:runnerId を処理し runner を削除する。
func (h *Handler) DeleteRunner(c *gin.Context) {
	runnerID := c.Param("runnerId")
	err := h.svc.DeregisterRunner(c.Request.Context(), runnerID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, model.CodeInternalError, "failed to deregister runner")
		return
	}
	c.Status(http.StatusNoContent)
}

// currentSessionId は session cookie 値そのままで、露出させると session hijack を許すため busy 一覧レスポンスには載せない。
type busyRunnerView struct {
	RunnerID string `json:"runnerId"`
}

type busyRunnersResponse struct {
	Runners []busyRunnerView `json:"runners"`
}

// GetListBusyRunners は GET /runners/busy を処理し busy runner の全件を返す。
func (h *Handler) GetListBusyRunners(c *gin.Context) {
	runners, err := h.svc.ListBusyRunners(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, model.CodeInternalError, "failed to list busy runners")
		return
	}
	views := make([]busyRunnerView, 0, len(runners))
	for _, r := range runners {
		views = append(views, busyRunnerView{RunnerID: r.RunnerID})
	}
	c.JSON(http.StatusOK, busyRunnersResponse{Runners: views})
}

// writeError はエラーレスポンスを JSON で返す。
func writeError(c *gin.Context, status int, code, message string) {
	reqID, _ := c.Get(requestIDKey)
	rid, _ := reqID.(string)
	c.JSON(status, model.ErrorResponse{Code: code, Message: message, RequestID: rid})
}
