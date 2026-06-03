package docker

import (
	"fmt"
	"net"
	"sync"
)

type PortManager struct {
	mu  sync.Mutex
	min int
	max int
}

func NewPortManager(min, max int) *PortManager {
	if min <= 0 || max <= 0 || max < min {
		min = 8000
		max = 9000
	}
	return &PortManager{min: min, max: max}
}

// Allocate probes the host and returns a free port.
func (pm *PortManager) Allocate() (int, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for port := pm.min; port <= pm.max; port++ {
		if isPortFreeOnHost(port) {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free ports in range %d-%d", pm.min, pm.max)
}

// AllocateSpecific checks a particular port on the host.
func (pm *PortManager) AllocateSpecific(port int) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if !isPortFreeOnHost(port) {
		return fmt.Errorf("port %d is in use on host", port)
	}
	return nil
}

// IsAvailable can also be simplified to just a host check.
func (pm *PortManager) IsAvailable(port int) bool {
	return isPortFreeOnHost(port)
}

func isPortFreeOnHost(port int) bool {
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	l.Close()
	return true
}