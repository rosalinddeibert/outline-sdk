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
	"sync/atomic"

	"github.com/Jigsaw-Code/outline-sdk/transport"
	psi "github.com/Psiphon-Labs/psiphon-tunnel-core/ClientLibrary/clientlib"
)

// The single [Dialer] we can have.
var singletonDialer = Dialer{
	startTunnel: startTunnel,
}

var (
	// TODO: check error usage
	errNotStartedDial       = errors.New("dialer has not been started yet")
	errNotStartedStop       = errors.New("tried to stop dialer that is not running")
	errTunnelTimeout        = errors.New("tunnel establishment timed out")
	errTunnelAlreadyStarted = errors.New("tunnel already started")
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
	tunnel atomic.Pointer[psi.PsiphonTunnel]
	// It is (and must be) okay for this function to be called multiple times concurrently
	// or in series.
	stop atomic.Pointer[func() error]
	dial func(context.Context, string) (transport.StreamConn, error)

	started atomic.Bool
	// Controls the Dialer state and Psiphon's global state.
	mu sync.Mutex
	// Used by Stop. After calling, the caller must set d.stop to nil.
	// Allows tests to override the tunnel creation.
	startTunnel func(ctx context.Context, config *DialerConfig) (*psi.PsiphonTunnel, error)
}

var _ transport.StreamDialer = (*Dialer)(nil)

// DialStream implements [transport.StreamDialer].
// The context is not used because Psiphon's implementation doesn't support it. If you need cancellation,
// you will need to add it independently.
func (d *Dialer) DialStream(unusedContext context.Context, addr string) (transport.StreamConn, error) {
	fmt.Println("*****************DialStream started")
	defer fmt.Println("*****************DialStream ended")
	tunnel := d.tunnel.Load()
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
	fmt.Println("*****************Start started")
	defer fmt.Println("*****************Start ended")
	if config == nil {
		return errors.New("config must not be nil")
	}

	// The mutex is locked only in this method, and is used to prevent concurrent calls to
	// Start from returning before the tunnel is started (or failed).
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.tunnel.Load() != nil {
		return errTunnelAlreadyStarted
	}
	// d.tunnel only gets set to a non-nil value in this function, and we're inside a
	// locked mutex, so we can be sure that it will remain nil until we set it below.

	startedTunnelSignal := make(chan struct{})
	defer close(startedTunnelSignal)
	tunnelCh := make(chan *psi.PsiphonTunnel)
	errCh := make(chan error)
	cancelCtx, cancel := context.WithCancel(ctx)

	// The stop function is not called from within a mutex.
	// It must be safe to call concurrently and multiple times.
	stop := func() error {
		cancel()
		// Wait for startTunnel to return (and note that it may return success or error at
		// this point).
		<-startedTunnelSignal
		tunnel := d.tunnel.Swap(nil)
		if tunnel == nil {
			return errNotStartedStop
		}
		// Only the first call to this function will actually get the tunnel and be able
		// to stop it; subsequent calls will get nil. (However, tunnel.Stop is itself
		// threadsafe, so it would be okay even if we called it multiple times or
		// concurrently.)
		tunnel.Stop()
		return nil
	}
	d.stop.Store(&stop)

	go func() {
		// In the case of an error being returned from StartTunnel, canceling the context
		// is the only cleanup necessary, and it can be done regardless.
		defer cancel()

		// StartTunnel returns when a tunnel is established or an error occurs.
		tunnel, err := d.startTunnel(cancelCtx, config)

		if err != nil {
			errCh <- err
		} else {
			tunnelCh <- tunnel
		}
	}()

	var err error
	select {
	case tunnel := <-tunnelCh:
		d.tunnel.Store(tunnel)
	case err = <-errCh:
	}

	if err != nil {
		// If there was an error, the context has already been canceled and there is no
		// more cleanup to be done.
		d.stop.Store(nil)

		if err == psi.ErrTimeout {
			// This can occur either because there was a timeout set in the tunnel config
			// or because the context deadline was exceeded.
			err = errTunnelTimeout
			if ctx.Err() == context.DeadlineExceeded {
				err = context.DeadlineExceeded
			}
		}

		return err
	}

	return nil
}

// Stop stops the Dialer background processes, releasing resources and allowing it to be reconfigured.
// It returns when the Dialer is completely stopped.
func (d *Dialer) Stop() error {
	fmt.Println("*****************Stop started")
	defer fmt.Println("*****************Stop ended")
	stop := d.stop.Swap(nil)
	if stop == nil {
		return errNotStartedStop
	}
	return (*stop)()
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
