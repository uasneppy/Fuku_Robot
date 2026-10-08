package shutdown

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/divkix/Alita_Robot/alita/utils/error_handling"
	log "github.com/sirupsen/logrus"
)

type Manager struct {
	handlers []func() error
	// timeouts is index-aligned with handlers; zero or less means the default allowance.
	timeouts        []time.Duration
	mu              sync.RWMutex
	once            sync.Once
	shutdownStarted atomic.Bool
}

var (
	notifySignals   = signal.Notify
	stopSignals     = signal.Stop
	exitProcess     = os.Exit
	shutdownTimeout = 60 * time.Second
	// defaultHandlerTimeout is the time a handler gets unless it was registered with
	// RegisterHandlerWithTimeout. The global shutdownTimeout still bounds it.
	defaultHandlerTimeout = 10 * time.Second
)

func NewManager() *Manager {
	return &Manager{
		handlers: make([]func() error, 0),
		timeouts: make([]time.Duration, 0),
	}
}

// RegisterHandler registers a shutdown handler that gets the default allowance
// (defaultHandlerTimeout). Handlers run in LIFO order.
func (m *Manager) RegisterHandler(handler func() error) {
	m.RegisterHandlerWithTimeout(handler, 0)
}

// RegisterHandlerWithTimeout registers a shutdown handler that gets timeout instead
// of the default allowance. A timeout of zero or less means the default. The global
// shutdown budget still bounds it: a handler can never keep shutdown running past it.
func (m *Manager) RegisterHandlerWithTimeout(handler func() error, timeout time.Duration) {
	if m.shutdownStarted.Load() {
		log.Warn("[Shutdown] RegisterHandler called after shutdown started - handler may not run")
	}
	if timeout > shutdownTimeout {
		log.Warnf("[Shutdown] Handler timeout %s exceeds the global shutdown budget %s and will be cut by it", timeout, shutdownTimeout)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers = append(m.handlers, handler)
	m.timeouts = append(m.timeouts, timeout)
}

func (m *Manager) WaitForShutdown() {
	sigChan := make(chan os.Signal, 1)
	notifySignals(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	sig := <-sigChan
	log.Infof("[Shutdown] Received signal: %v", sig)

	stopSignals(sigChan)
	close(sigChan)

	m.shutdown()
}

func (m *Manager) executeHandler(handler func() error, index int) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("[Shutdown] Handler %d panicked: %v", index, r)
		}
	}()
	return handler()
}

// runHandlers runs every registered handler in LIFO order under a global budget and
// returns the process exit code: 1 when the budget ran out, 0 otherwise. Each handler's
// context derives from the budget context, so its own timeout can never outlast it.
func (m *Manager) runHandlers(budget, defaultTimeout time.Duration) int {
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()

	m.mu.Lock()
	handlers := make([]func() error, len(m.handlers))
	copy(handlers, m.handlers)
	timeouts := make([]time.Duration, len(m.timeouts))
	copy(timeouts, m.timeouts)
	m.mu.Unlock()

	for i := len(handlers) - 1; i >= 0; i-- {
		allowance := defaultTimeout
		if i < len(timeouts) && timeouts[i] > 0 {
			allowance = timeouts[i]
		}
		hCtx, hCancel := context.WithTimeout(ctx, allowance)
		done := make(chan error, 1)
		go func(h func() error, idx int) {
			done <- m.executeHandler(h, idx)
		}(handlers[i], i)

		select {
		case <-hCtx.Done():
			log.Warnf("[Shutdown] Handler %d timed out after %s, skipping", i, allowance)
		case err := <-done:
			if err != nil {
				log.Errorf("[Shutdown] Handler %d error: %v", i, err)
			}
		}
		hCancel()

		if ctx.Err() != nil {
			log.Warn("[Shutdown] Global timeout, forcing exit")
			return 1
		}
	}
	return 0
}

func (m *Manager) shutdown() {
	m.once.Do(func() {
		defer error_handling.RecoverFromPanic("shutdown", "shutdown")

		m.shutdownStarted.Store(true)
		log.Info("[Shutdown] Starting graceful shutdown...")

		code := m.runHandlers(shutdownTimeout, defaultHandlerTimeout)
		if code == 0 {
			log.Info("[Shutdown] Graceful shutdown completed")
		}
		exitProcess(code)
	})
}
