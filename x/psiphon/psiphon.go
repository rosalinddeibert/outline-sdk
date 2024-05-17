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

// Package psiphon provides adaptors to use Psiphon as a StreamDialer.
//
// You will need to provide a [Psiphon config file].
//
// [Psiphon config file]: https://github.com/Psiphon-Labs/psiphon-tunnel-core/tree/master?tab=readme-ov-file#generate-configuration-data
package psiphon

import (
	"context"
	"net"

	"github.com/Jigsaw-Code/outline-sdk/transport"
	psi "github.com/Psiphon-Labs/psiphon-tunnel-core/ClientLibrary/clientlib"
)

type PsiphonDialer struct {
	tunnel *psi.PsiphonTunnel
}

// NewStreamDialer creates a new Psiphon StreamDialer. It returns either when an error
// occurs (including a configured timeout) or a Psiphon tunnel is established.
//
// The embeddedServerEntryList is a string that contains the initial Psiphon server
// entries. If this is not supplied, a server list will be fetched before the first
// connection attempt.
//
// You may either add your config customizations to the JSON or use the psi.Parameters
// struct along with the exact config JSON provided by Psiphon.
//
//   - DataRootDirectory: The directory where Psiphon will store its data. Required.
//
//   - ClientPlatform: This can be as simple as "outline", or it can capture more
//     information with underscore-delimited suffixes, like:
//     "outline_<version>_<integrator>". These extra fields can be used to provide more
//     detailed information in usage stats (available to Psiphon Inc.).
//     Required if not already set in the config JSON.
//
//   - NetworkID: Can be "WIFI" or "MOBILE". Optional.
//
//   - EstablishTunnelTimeoutSeconds: Do not use. (It is set to 0 in the config and should
//     not be overridden.)
//
//   - EmitDiagnosticNoticesToFiles: Only set to true when debugging Psiphon behaviour.
//
//   - DisableLocalSocksProxy, DisableLocalHTTPProxy: Do not use. Set by this function to
//     prevent Psiphon from running local proxies.
func NewStreamDialer(ctx context.Context, configJSON []byte, embeddedServerEntryList string, params psi.Parameters) (*PsiphonDialer, error) {
	// We access the Psiphon tunnel directly through a dialer, so we have
	// no need for local proxies.
	t := true
	params.DisableLocalSocksProxy = &t
	params.DisableLocalHTTPProxy = &t

	tunnel, err := psi.StartTunnel(ctx, configJSON, embeddedServerEntryList, params, nil, nil)
	if err != nil {
		return nil, err
	}

	return &PsiphonDialer{tunnel}, nil
}

// DialStream establishes a connection to addr through the Psiphon tunnel.
func (d *PsiphonDialer) DialStream(ctx context.Context, addr string) (transport.StreamConn, error) {
	netConn, err := d.tunnel.Dial(addr)
	if err != nil {
		return nil, err
	}
	return streamConn{netConn}, nil
}

// Close stops the Psiphon tunnel. Safe to call even if the tunnel is not established.
func (d *PsiphonDialer) Close() {
	d.tunnel.Stop()
}

var _ transport.StreamDialer = (*PsiphonDialer)(nil)

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
