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

package clientlib

import (
	"context"
	"errors"
	"net"
)

// See https://pkg.go.dev/github.com/Psiphon-Labs/psiphon-tunnel-core/ClientLibrary/clientlib

type PsiphonTunnel struct{}

type Parameters struct {
	DataRootDirectory             *string
	ClientPlatform                *string
	NetworkID                     *string
	EstablishTunnelTimeoutSeconds *int
	EmitDiagnosticNoticesToFiles  bool
	DisableLocalSocksProxy        *bool
	DisableLocalHTTPProxy         *bool
}

type ParametersDelta map[string]interface{}

type NoticeEvent struct{}

func StartTunnel(
	ctx context.Context,
	configJSON []byte,
	embeddedServerEntryList string,
	params Parameters,
	paramsDelta ParametersDelta,
	noticeReceiver func(NoticeEvent)) (retTunnel *PsiphonTunnel, retErr error) {
	return nil, errors.New("not available")
}

func (tunnel *PsiphonTunnel) Stop() {
}

func (tunnel *PsiphonTunnel) Dial(remoteAddr string) (conn net.Conn, err error) {
	return nil, errors.New("not available")
}
