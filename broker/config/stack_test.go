package config

import (
	"reflect"
	"strings"
	"testing"
)

// TestNewStackFromEnv_Success は STACK_NAME と BUNSHIN_STACKS が揃った場合に self と fallback を返すことを検証する。
func TestNewStackFromEnv_Success(t *testing.T) {
	t.Setenv("STACK_NAME", "ap-northeast-1")
	t.Setenv("BUNSHIN_STACKS", "ap-northeast-1,ap-northeast-3")

	got, err := NewStackFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Self != "ap-northeast-1" {
		t.Errorf("Self = %q, want %q", got.Self, "ap-northeast-1")
	}
	if !reflect.DeepEqual(got.Fallbacks, []string{"ap-northeast-3"}) {
		t.Errorf("Fallbacks = %v, want [ap-northeast-3]", got.Fallbacks)
	}
}

// TestNewStackFromEnv_SingleStackNoFallback は自 stack のみを列挙した場合に fallback が空になることを検証する。
func TestNewStackFromEnv_SingleStackNoFallback(t *testing.T) {
	t.Setenv("STACK_NAME", "ap-northeast-1")
	t.Setenv("BUNSHIN_STACKS", "ap-northeast-1")

	got, err := NewStackFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Fallbacks) != 0 {
		t.Errorf("Fallbacks = %v, want empty", got.Fallbacks)
	}
}

// TestNewStackFromEnv_MissingStack は STACK_NAME が未設定・空白のみ時に
// いずれも "missing" エラーとして扱われることを検証する。
func TestNewStackFromEnv_MissingStack(t *testing.T) {
	tests := []struct {
		name  string
		stack string
	}{
		{"empty", ""},
		{"whitespace only", "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("STACK_NAME", tt.stack)
			t.Setenv("BUNSHIN_STACKS", "ap-northeast-1")

			_, err := NewStackFromEnv()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), "missing required environment variable: STACK_NAME") {
				t.Errorf("error = %q, want missing STACK_NAME", err.Error())
			}
		})
	}
}

// TestNewStackFromEnv_MissingStacks は BUNSHIN_STACKS が未設定・空白のみ時に
// いずれも "missing" エラーとして扱われることを検証する。
func TestNewStackFromEnv_MissingStacks(t *testing.T) {
	tests := []struct {
		name  string
		stack string
	}{
		{"empty", ""},
		{"whitespace only", "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("STACK_NAME", "ap-northeast-1")
			t.Setenv("BUNSHIN_STACKS", tt.stack)

			_, err := NewStackFromEnv()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), "missing required environment variable: BUNSHIN_STACKS") {
				t.Errorf("error = %q, want missing BUNSHIN_STACKS", err.Error())
			}
		})
	}
}

// TestNewStackFromEnv_StackNotInList は STACK_NAME が BUNSHIN_STACKS の列挙外なら起動失敗することを検証する。
func TestNewStackFromEnv_StackNotInList(t *testing.T) {
	t.Setenv("STACK_NAME", "ap-northeast-1")
	t.Setenv("BUNSHIN_STACKS", "ap-northeast-2,ap-northeast-3")

	_, err := NewStackFromEnv()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "BUNSHIN_STACKS") {
		t.Errorf("error = %q, want to mention BUNSHIN_STACKS", err.Error())
	}
}

// TestNewStackFromEnv_TrimsWhitespace は STACK_NAME / BUNSHIN_STACKS 双方の要素が
// 前後空白付きでも trim 後の値で一致判定され、fallback もトリム済みで返ることを検証する。
func TestNewStackFromEnv_TrimsWhitespace(t *testing.T) {
	t.Setenv("STACK_NAME", " ap-northeast-3 ")
	t.Setenv("BUNSHIN_STACKS", " ap-northeast-1 , ap-northeast-3 ")

	got, err := NewStackFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Self != "ap-northeast-3" {
		t.Errorf("Self = %q, want %q", got.Self, "ap-northeast-3")
	}
	if !reflect.DeepEqual(got.Fallbacks, []string{"ap-northeast-1"}) {
		t.Errorf("Fallbacks = %v, want [ap-northeast-1]", got.Fallbacks)
	}
}
