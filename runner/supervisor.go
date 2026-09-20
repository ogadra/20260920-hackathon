package main

import (
	"context"
	"log"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const (
	perlAppRestartDelay  = 500 * time.Millisecond
	perlAppShutdownGrace = 5 * time.Second
)

func supervise(ctx context.Context, name string, factory func() *exec.Cmd, restartDelay, shutdownGrace time.Duration) {
	for ctx.Err() == nil {
		superviseOnce(ctx, name, factory, shutdownGrace)
		if ctx.Err() != nil {
			return
		}
		log.Printf("supervisor: %s restarting in %s", name, restartDelay)
		if !supervisorSleep(ctx, restartDelay) {
			return
		}
	}
}

// superviseOnceはfactoryが返すexec.CmdがSysProcAttr{Setpgid: true}を持つことを前提にする。
// perl親がaccept per connectionでforkする子ハンドラをまとめて止めるため、cmd.Process.Signalではなく
// syscall.Kill(-pid, sig)でプロセスグループ全体へsignalを送る (pgid == pidは独立グループ化の効果)。
func superviseOnce(ctx context.Context, name string, factory func() *exec.Cmd, shutdownGrace time.Duration) {
	cmd := factory()
	if err := cmd.Start(); err != nil {
		log.Printf("supervisor: %s start: %v", name, err)
		return
	}
	pid := cmd.Process.Pid
	log.Printf("supervisor: %s started pid=%d", name, pid)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		log.Printf("supervisor: %s exited: %v", name, err)
	case <-ctx.Done():
		log.Printf("supervisor: %s sending SIGTERM to pgid=%d", name, pid)
		_ = syscall.Kill(-pid, syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(shutdownGrace):
			log.Printf("supervisor: %s SIGTERM timed out after %s, sending SIGKILL to pgid=%d", name, shutdownGrace, pid)
			_ = syscall.Kill(-pid, syscall.SIGKILL)
			<-done
		}
	}
}

func supervisorSleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func perlAppFactory() *exec.Cmd {
	c := exec.Command("perl", "/app/server.pl")
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	// superviseOnceがsyscall.Kill(-pid, ...)でforked child connectionハンドラも巻き取れるよう
	// 独立プロセスグループにする (pgid == pid)。
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return c
}

func runAppSupervisor(ctx context.Context) {
	supervise(ctx, "perl-app", perlAppFactory, perlAppRestartDelay, perlAppShutdownGrace)
}

var runAppSupervisorFn = runAppSupervisor
