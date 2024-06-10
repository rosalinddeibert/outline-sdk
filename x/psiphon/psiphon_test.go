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

//go:build psiphon

package psiphon

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	psi "github.com/Psiphon-Labs/psiphon-tunnel-core/ClientLibrary/clientlib"
	"github.com/stretchr/testify/require"
)

func newTestConfig(tb testing.TB) (*DialerConfig, func()) {
	tempDir, err := os.MkdirTemp("", "psiphon")
	require.NoError(tb, err)
	return &DialerConfig{
		DataRootDirectory: tempDir,
		ProviderConfig: json.RawMessage(`{
			"PropagationChannelId": "ID1",
			"SponsorId": "ID2"
		}`),
	}, func() { os.RemoveAll(tempDir) }
}

func TestDialer_StartSuccessful(t *testing.T) {
	// Create minimal config.
	cfg, delete := newTestConfig(t)
	defer delete()

	dialer := GetSingletonDialer()

	// Set a no-op startTunnel
	dialer.startTunnel = func(ctx context.Context, config *DialerConfig) (*psi.PsiphonTunnel, error) {
		return &psi.PsiphonTunnel{}, nil
	}
	defer func() {
		dialer.startTunnel = startTunnel
	}()

	errCh := make(chan error)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		errCh <- dialer.Start(ctx, cfg)
	}()

	err := <-errCh
	require.NoError(t, err)
	require.NoError(t, dialer.Stop())
}

func TestDialerStart_Cancelled(t *testing.T) {
	cfg, delete := newTestConfig(t)
	defer delete()
	errCh := make(chan error)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		errCh <- GetSingletonDialer().Start(ctx, cfg)
	}()
	cancel()
	err := <-errCh
	GetSingletonDialer().Stop()
	require.ErrorIs(t, err, context.Canceled)
}

func TestDialerStart_Timeout(t *testing.T) {
	cfg, delete := newTestConfig(t)
	defer delete()
	errCh := make(chan error)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now())
	defer cancel()
	go func() {
		errCh <- GetSingletonDialer().Start(ctx, cfg)
	}()
	err := <-errCh
	GetSingletonDialer().Stop()
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestDialerDialStream_NotStarted(t *testing.T) {
	_, err := GetSingletonDialer().DialStream(context.Background(), "")
	require.ErrorIs(t, err, errNotStartedDial)
}

func TestDialerStop_NotStarted(t *testing.T) {
	err := GetSingletonDialer().Stop()
	require.ErrorIs(t, err, errNotStartedStop)
}
