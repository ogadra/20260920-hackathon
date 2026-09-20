// Package stacklist は BUNSHIN_STACKS / X-Fallback-Remaining のカンマ区切り stack 値を
// パースする下位ユーティリティを提供する。
// handler (HTTP 層) と config (composition-root) の双方から下位方向として import できるよう、
// どちらのパッケージにも属さない独立パッケージにしている。
package stacklist

import "strings"

// Split はカンマ区切り値を stack 名の並びに分割する。前後空白は削り、空要素は捨てる。
func Split(raw string) []string {
	var stacks []string
	for _, s := range strings.Split(raw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			stacks = append(stacks, s)
		}
	}
	return stacks
}

// Parse は raw を走査し、self を pivot にした周回順の fallback 一覧と、
// self が列挙に含まれていたかを返す。周回は「self の直後 → 末尾 → 先頭 → self の直前」で、
// self 自身は結果から除外する。self が列挙に無い場合は raw の並びをそのまま返す。
//
// 単純に self を除いた入力順を返すと、全 stack で BUNSHIN_STACKS を共有した構成では
// 一発目 fallback 先が全 stack で同一になり、特定 stack へ枯渇が集中する。
// 周回によって stack ごとに並びをずらし、fallback 先を均等に分散させる。
//
// self 判定と周回抽出を別関数に分けると format 変更時に片方だけ更新される
// リスクがあるため、両操作を単一関数に集約している。
func Parse(raw, self string) ([]string, bool) {
	all := Split(raw)
	pivot := -1
	for i, s := range all {
		if s == self {
			pivot = i
			break
		}
	}
	fallbacks := []string{}
	if pivot == -1 {
		fallbacks = append(fallbacks, all...)
		return fallbacks, false
	}
	for i := pivot + 1; i < len(all); i++ {
		if all[i] == self {
			continue
		}
		fallbacks = append(fallbacks, all[i])
	}
	fallbacks = append(fallbacks, all[:pivot]...)
	return fallbacks, true
}
