package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
)

// ErrShellNotFound is returned when the requested shell ID does not exist.
var ErrShellNotFound = errors.New("shell not found")

// ShellManager manages multiple persistent bash shells keyed by shell ID.
// It is safe for concurrent use.
type ShellManager struct {
	mu       sync.Mutex
	shells   map[string]Shell
	genID    func() (string, error) // ID generator; defaults to generateID
	newShell func() (Shell, error)  // shell factory; defaults to newDefaultShell
}

// newDefaultShell wraps NewBashShell to satisfy the Shell factory signature.
func newDefaultShell() (Shell, error) {
	return NewBashShell()
}

// NewShellManager creates an empty ShellManager.
func NewShellManager() *ShellManager {
	return &ShellManager{
		shells:   make(map[string]Shell),
		genID:    generateID,
		newShell: newDefaultShell,
	}
}

// generateID returns a cryptographically random 16-byte hex string of 32 characters.
// crypto/rand.Read always returns len(b) and a nil error on supported platforms,
// so the error return exists only to satisfy the genID function signature for testability.
func generateID() (string, error) {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b), nil
}

// Create starts a new bash shell and returns its ID and the Shell.
func (m *ShellManager) Create() (string, Shell, error) {
	id, err := m.genID()
	if err != nil {
		return "", nil, err
	}

	shell, err := m.newShell()
	if err != nil {
		return "", nil, fmt.Errorf("create shell: %w", err)
	}

	m.mu.Lock()
	if _, exists := m.shells[id]; exists {
		m.mu.Unlock()
		_ = shell.Close()
		return "", nil, fmt.Errorf("create shell: duplicate shell id %q", id)
	}
	m.shells[id] = shell
	m.mu.Unlock()

	return id, shell, nil
}

// Get returns the Shell for the given shell ID.
// Returns an error if the shell does not exist.
func (m *ShellManager) Get(id string) (Shell, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	shell, ok := m.shells[id]
	if !ok {
		return nil, ErrShellNotFound
	}
	return shell, nil
}

// Delete closes and removes the shell with the given ID.
// Returns an error if the shell does not exist.
func (m *ShellManager) Delete(id string) error {
	m.mu.Lock()
	shell, ok := m.shells[id]
	if !ok {
		m.mu.Unlock()
		return ErrShellNotFound
	}
	delete(m.shells, id)
	m.mu.Unlock()

	return shell.Close()
}

// CloseAll closes all shells and clears the map.
// Returns the first error encountered, but attempts to close all shells.
func (m *ShellManager) CloseAll() error {
	m.mu.Lock()
	shells := m.shells
	m.shells = make(map[string]Shell)
	m.mu.Unlock()

	var firstErr error
	for _, shell := range shells {
		if err := shell.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
