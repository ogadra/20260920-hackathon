package stacklist

import (
	"reflect"
	"testing"
)

// TestSplit はカンマ区切りの分解・空要素除去・前後空白トリムを検証する。
func TestSplit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"empty", "", nil},
		{"whitespace only", "   ", nil},
		{"only commas", ",,,", nil},
		{"single", "ap-northeast-1", []string{"ap-northeast-1"}},
		{"trimmed and dropped empties", " ap-northeast-1 , ,ap-northeast-3", []string{"ap-northeast-1", "ap-northeast-3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Split(tt.raw); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Split(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

// TestParse は self を pivot にした周回順の fallback 抽出と self 検出を同時に検証する。
func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		raw              string
		self             string
		wantFallbacks    []string
		wantContainsSelf bool
	}{
		{"empty input", "", "ap-northeast-1", []string{}, false},
		{"self absent", "ap-northeast-2,ap-northeast-3", "ap-northeast-1", []string{"ap-northeast-2", "ap-northeast-3"}, false},
		{"self at head", " ap-northeast-1 , ,ap-northeast-3,ap-northeast-2", "ap-northeast-1", []string{"ap-northeast-3", "ap-northeast-2"}, true},
		{"self in middle rotates", "ap-northeast-1,ap-northeast-3,asia-northeast1,asia-northeast2", "ap-northeast-3", []string{"asia-northeast1", "asia-northeast2", "ap-northeast-1"}, true},
		{"self at tail rotates", "ap-northeast-1,ap-northeast-3,asia-northeast1,asia-northeast2", "asia-northeast2", []string{"ap-northeast-1", "ap-northeast-3", "asia-northeast1"}, true},
		{"self present multiple times", "ap-northeast-1,ap-northeast-3,ap-northeast-1", "ap-northeast-1", []string{"ap-northeast-3"}, true},
		{"only self", "ap-northeast-1", "ap-northeast-1", []string{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotFallbacks, gotContainsSelf := Parse(tt.raw, tt.self)
			if !reflect.DeepEqual(gotFallbacks, tt.wantFallbacks) {
				t.Errorf("fallbacks = %v, want %v", gotFallbacks, tt.wantFallbacks)
			}
			if gotContainsSelf != tt.wantContainsSelf {
				t.Errorf("containsSelf = %v, want %v", gotContainsSelf, tt.wantContainsSelf)
			}
		})
	}
}
