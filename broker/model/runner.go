// Package model はドメインモデルを提供する。
package model

type State string

const (
	StateIdle State = "idle"
	StateBusy State = "busy"
)

type Runner struct {
	RunnerID         string `dynamodbav:"runnerId"`
	State            State  `dynamodbav:"state,omitempty"`
	CurrentSessionID string `dynamodbav:"currentSessionId,omitempty"`
	PrivateHost      string `dynamodbav:"privateHost,omitempty"`
}

func (r *Runner) IsIdle() bool {
	return r.State == StateIdle
}

func (r *Runner) IsBusy() bool {
	return r.State == StateBusy
}
