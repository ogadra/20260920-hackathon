// Package model はドメインモデルのテストを提供する。
package model

import "testing"

func TestRunner_IsIdle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		runner Runner
		want   bool
	}{
		{
			name:   "idle when state is idle",
			runner: Runner{RunnerID: "r1", State: StateIdle},
			want:   true,
		},
		{
			name:   "idle when state is idle with privateHost",
			runner: Runner{RunnerID: "r1", State: StateIdle, PrivateHost: "10.0.0.1"},
			want:   true,
		},
		{
			name:   "not idle when state is busy",
			runner: Runner{RunnerID: "r1", State: StateBusy, CurrentSessionID: "sess-1"},
			want:   false,
		},
		{
			name:   "not idle when state is empty",
			runner: Runner{RunnerID: "r1"},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.runner.IsIdle(); got != tt.want {
				t.Errorf("IsIdle() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRunner_IsBusy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		runner Runner
		want   bool
	}{
		{
			name:   "busy when state is busy",
			runner: Runner{RunnerID: "r1", State: StateBusy, CurrentSessionID: "sess-1"},
			want:   true,
		},
		{
			name:   "busy when state is busy with privateHost",
			runner: Runner{RunnerID: "r1", State: StateBusy, CurrentSessionID: "sess-1", PrivateHost: "10.0.0.1"},
			want:   true,
		},
		{
			name:   "not busy when state is idle",
			runner: Runner{RunnerID: "r1", State: StateIdle},
			want:   false,
		},
		{
			name:   "not busy when state is empty",
			runner: Runner{RunnerID: "r1"},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.runner.IsBusy(); got != tt.want {
				t.Errorf("IsBusy() = %v, want %v", got, tt.want)
			}
		})
	}
}
