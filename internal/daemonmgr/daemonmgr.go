package daemonmgr

import (
	"log"
	"sync"

	"github.com/goppydae/gapi/internal/eventbus"
	"github.com/goppydae/gapi/internal/lifecycle"
	"github.com/goppydae/gapi/internal/procdaemon"
)

type DaemonManager struct {
	mu      sync.RWMutex
	daemons map[string]lifecycle.Daemon
	bus     *eventbus.EventBus
}

func NewDaemonManager(bus *eventbus.EventBus) *DaemonManager {
	return &DaemonManager{
		daemons: make(map[string]lifecycle.Daemon),
		bus:     bus,
	}
}

// RegisterProc creates a gRPC-backed ProcDaemon and registers it
func (dm *DaemonManager) RegisterProc(id, lang, scope, topicRoot, grpcAddr string) error {
	pd, err := procdaemon.NewProcDaemon(id, lang, scope, topicRoot, grpcAddr, dm.bus)
	if err != nil {
		return err
	}
	return dm.RegisterDaemon(pd)
}

// RegisterDaemon registers a daemon and binds control topics
func (dm *DaemonManager) RegisterDaemon(d lifecycle.Daemon) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	id := d.ID()
	scope := d.Scope()
	root := id // topic root defaults to ID

	dm.daemons[id] = d
	log.Printf("Daemon registered: [%s] scope=%s type=%s", id, scope, d.Type())

	// Bind control topic listeners
	_ = dm.bus.Subscribe(scope, root+"/control.start", func(e eventbus.Event) {
		if err := d.Start(); err != nil {
			log.Printf("Error starting daemon [%s]: %v", id, err)
		}
	})
	_ = dm.bus.Subscribe(scope, root+"/control.stop", func(e eventbus.Event) {
		if err := d.Stop(); err != nil {
			log.Printf("Error stopping daemon [%s]: %v", id, err)
		}
	})
	_ = dm.bus.Subscribe(scope, root+"/control.reload", func(e eventbus.Event) {
		if err := d.Reload(); err != nil {
			log.Printf("Error reloading daemon [%s]: %v", id, err)
		}
	})

	return nil
}

// StopAll sends stop signals to all registered daemons and closes transports if applicable
func (dm *DaemonManager) StopAll() {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	var wg sync.WaitGroup
	for id, d := range dm.daemons {
		wg.Add(1)
		go func(id string, d lifecycle.Daemon) {
			defer wg.Done()

			if err := d.Stop(); err != nil {
				log.Printf("Error stopping daemon [%s]: %v", id, err)
			}

			// Check if daemon implements Closeable transport interface and close if applicable
			if tc, ok := d.(interface{ Close() error }); ok {
				if err := tc.Close(); err != nil {
					log.Printf("Error closing transport for daemon [%s]: %v", id, err)
				} else {
					log.Printf("Transport closed gracefully for daemon [%s]", id)
				}
			}
		}(id, d)
	}
	wg.Wait()
	log.Println("All daemons stopped and transports closed gracefully.")
}

// Describe returns summaries of all registered daemons
func (dm *DaemonManager) Describe() []map[string]string {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	var out []map[string]string
	for _, d := range dm.daemons {
		out = append(out, d.Describe())
	}
	return out
}

// Get returns a daemon by ID
func (dm *DaemonManager) Get(id string) lifecycle.Daemon {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.daemons[id]
}
