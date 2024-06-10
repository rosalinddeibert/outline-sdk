// Copyright 2024 Jigsaw Operations LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package psiphon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/Jigsaw-Code/outline-sdk/transport"
	psi "github.com/Psiphon-Labs/psiphon-tunnel-core/ClientLibrary/clientlib"
)

// The single [Dialer] we can have.
var singletonDialer = Dialer{
	startTunnel: startTunnel,
}

var (
	errNotStartedDial = errors.New("dialer has not been started yet")
	errNotStartedStop = errors.New("tried to stop dialer that is not running")
	errTunnelTimeout  = errors.New("tunnel establishment timed out")
)

// DialerConfig specifies the parameters for [Dialer].
type DialerConfig struct {
	// Used as the directory for the datastore, remote server list, and obfuscasted
	// server list.
	// Empty string means the default will be used (current working directory).
	// Strongly recommended.
	DataRootDirectory string

	// Raw JSON config provided by Psiphon.
	ProviderConfig []byte
}

// Dialer is a [transport.StreamDialer] that uses Psiphon to connect to a destination.
// There's only one possible Psiphon Dialer available at any time, which is accessible via [GetSingletonDialer].
// The zero value of this type is invalid.
//
// The Dialer must be configured first with [Dialer.Start] before it can be used, and [Dialer.Stop] must be
// called before you can start it again with a new configuration. Dialer.Stop should be called
// when you no longer need the Dialer in order to release resources.
type Dialer struct {
	// Controls the Dialer state and Psiphon's global state.
	mu sync.Mutex
	// Used by DialStream.
	tunnel *psi.PsiphonTunnel
	// Used by Stop. After calling, the caller must set d.stop and d.tunnel to nil.
	stop func()
	// Allows tests to override the tunnel creation.
	startTunnel func(ctx context.Context, config *DialerConfig) (*psi.PsiphonTunnel, error)
}

var _ transport.StreamDialer = (*Dialer)(nil)

// DialStream implements [transport.StreamDialer].
// The context is not used because Psiphon's implementation doesn't support it. If you need cancellation,
// you will need to add it independently.
func (d *Dialer) DialStream(unusedContext context.Context, addr string) (transport.StreamConn, error) {
	d.mu.Lock()
	tunnel := d.tunnel
	d.mu.Unlock()
	if tunnel == nil {
		return nil, errNotStartedDial
	}
	netConn, err := tunnel.Dial(addr)
	if err != nil {
		return nil, err
	}
	return streamConn{netConn}, nil
}

func startTunnel(ctx context.Context, config *DialerConfig) (*psi.PsiphonTunnel, error) {
	// Disable Psiphon's local proxy servers, which we don't use.
	// Note that these parameters override anything in the provider config.
	trueValue := true
	params := psi.Parameters{
		DataRootDirectory:      &config.DataRootDirectory,
		DisableLocalSocksProxy: &trueValue,
		DisableLocalHTTPProxy:  &trueValue,
	}

	return psi.StartTunnel(ctx, config.ProviderConfig, "", params, nil, nil)
}

// Start configures and runs the Dialer. It must be called before you can use the Dialer. It returns when the tunnel is ready for use.
func (d *Dialer) Start(ctx context.Context, config *DialerConfig) error {
	if config == nil {
		return errors.New("config must not be nil")
	}

	// Ensure that the tunnel establishment can be interrupted by Stop.
	cancelCtx, cancel := context.WithCancel(ctx)
	d.mu.Lock()
	if d.stop != nil {
		d.mu.Unlock()
		return errors.New("tried to start dialer that is already running")
	}
	d.stop = cancel
	d.mu.Unlock()

	// StartTunnel returns when a tunnel is established or an error occurs.
	tunnel, err := d.startTunnel(cancelCtx, config)
	if err != nil {
		if err == psi.ErrTimeout {
			// This can occur either because there was a timeout set in the tunnel config
			// or because the context deadline was exceeded.
			err = errTunnelTimeout
			if ctx.Err() == context.DeadlineExceeded {
				err = context.DeadlineExceeded
			}
		}
		cancel()
		return fmt.Errorf("psi.StartTunnel failed: %w", err)
	}

	d.mu.Lock()
	d.tunnel = tunnel
	// Now that we have a running tunnel, we need to include stopping it in our stop function.
	d.stop = func() {
		cancel()
		// Waits until the tunnel is stopped.
		tunnel.Stop()
	}
	d.mu.Unlock()

	return nil
}

// Stop stops the Dialer background processes, releasing resources and allowing it to be reconfigured.
// It returns when the Dialer is completely stopped.
func (d *Dialer) Stop() error {
	// Holding a lock while calling the stop function ensures that any concurrent call
	// to Stop will wait for the first call to finish before returning, rather than
	// returning immediately (because tunnel.stop is nil) and thereby indicating
	// (erroneously) that the tunnel has been stopped.
	// Stopping a tunnel happens quickly enough that this processing block shouldn't be
	// a problem.
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stop == nil {
		return errNotStartedStop
	}
	d.stop()
	d.stop = nil
	d.tunnel = nil
	return nil
}

// GetSingletonDialer returns the single Psiphon dialer instance.
func GetSingletonDialer() *Dialer {
	return &singletonDialer
}

// streamConn wraps a [net.Conn] to provide a [transport.StreamConn] interface.
type streamConn struct {
	net.Conn
}

var _ transport.StreamConn = (*streamConn)(nil)

func (c streamConn) CloseWrite() error {
	return nil
}

func (c streamConn) CloseRead() error {
	return nil
}
