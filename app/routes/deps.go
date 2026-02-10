package routes

import (
	"sync"

	chatsvc "rhone_chat/internal/services/chat"
)

type Deps struct {
	Chat *chatsvc.Service
}

var (
	depsMu   sync.RWMutex
	depsOnce bool
	deps     Deps
)

func SetDeps(next Deps) {
	depsMu.Lock()
	defer depsMu.Unlock()
	deps = next
	depsOnce = true
}

func getDeps() Deps {
	depsMu.RLock()
	defer depsMu.RUnlock()
	if !depsOnce {
		panic("routes deps not initialized")
	}
	return deps
}
