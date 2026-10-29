package privacyhost

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNilClientIsUnavailableAndSafeToClose(t *testing.T) {
	var client *Client
	if !client.Closed() {
		t.Fatal("nil client claimed a usable transport")
	}
	for i := 0; i < 2; i++ {
		if err := client.Close(); err != nil {
			t.Fatal("nil resource cleanup failed", err)
		}
	}
	if err := client.Call(context.Background(), "Report", nil, nil); !errors.Is(err, net.ErrClosed) {
		t.Fatal("nil client did not refuse a call", err)
	}
}

func TestConcurrentClientCloseInterruptsCallAndReleasesOnce(t *testing.T) {
	ui, helper := net.Pipe() // In-memory streams, no network listener/socket.
	defer helper.Close()
	var releases atomic.Int32
	client := newClient(ui, strings.Repeat("f", 64), func() { releases.Add(1) })
	defer client.Close()
	received := make(chan error, 1)
	go func() { var req request; received <- readFrame(helper, &req, maxRequest) }()
	done := make(chan error, 1)
	go func() { done <- client.Call(context.Background(), "Report", nil, nil) }()
	select {
	case err := <-received:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("in-memory request was not received")
	}
	var workers sync.WaitGroup
	for i := 0; i < 32; i++ {
		workers.Add(1)
		go func() { defer workers.Done(); _ = client.Close() }()
	}
	workers.Wait()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("closed call claimed success without a reply")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("closing the client did not interrupt the in-flight call")
	}
	if !client.Closed() || releases.Load() != 1 {
		t.Fatal("concurrent close did not release the transport exactly once", releases.Load())
	}
	if err := client.Call(context.Background(), "Report", nil, nil); !errors.Is(err, net.ErrClosed) || releases.Load() != 1 {
		t.Fatal("closed session reopened or repeated cleanup", err)
	}
}
