package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/ogadra/bunshin/broker/stacklist"
)

// StackConfig は broker 自身の stack 識別子と fallback 転送先を保持する。
type StackConfig struct {
	// Self は自 stack 名。session ID の prefix にも使われる。
	Self string
	// Fallbacks は BUNSHIN_STACKS を Self 起点で周回し Self を除いた転送候補。
	// 先頭が一発目の fallback 先。空の場合は fallback を発行しない。
	Fallbacks []string
}

// NewStackFromEnv は STACK_NAME と BUNSHIN_STACKS を読んで StackConfig を組み立てる。
// STACK_NAME と BUNSHIN_STACKS は必須で、STACK_NAME は BUNSHIN_STACKS に列挙されている必要がある。
// "" と 空白のみ の入力に対して失敗経路を分岐させると設定ミスの原因を切り分けにくいため、
// TrimSpace 後の空文字は一律「missing」として扱う。
func NewStackFromEnv() (StackConfig, error) {
	self := strings.TrimSpace(os.Getenv("STACK_NAME"))
	if self == "" {
		return StackConfig{}, fmt.Errorf("missing required environment variable: STACK_NAME")
	}
	rawList := os.Getenv("BUNSHIN_STACKS")
	if strings.TrimSpace(rawList) == "" {
		return StackConfig{}, fmt.Errorf("missing required environment variable: BUNSHIN_STACKS")
	}
	fallbacks, containsSelf := stacklist.Parse(rawList, self)
	if !containsSelf {
		return StackConfig{}, fmt.Errorf("STACK_NAME %q is not listed in BUNSHIN_STACKS %q", self, rawList)
	}
	return StackConfig{
		Self:      self,
		Fallbacks: fallbacks,
	}, nil
}
