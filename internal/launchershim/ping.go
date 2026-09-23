package launchershim

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// Ping is what a running shim sends, best-effort, the moment it starts a game -
// mirrors the small payload companions/launcher-shim/main.go's own pingParallax
// writes.
type Ping struct {
	ResolvedExe string `json:"resolvedExe"`
	PID         int    `json:"pid"`
}

// StartPingListener binds the fixed loopback port a running shim will try to reach
// (see PingPort) and calls onPing for every one it receives, until ctx is
// cancelled. If the port is already in use (another instance of this app, or an
// unrelated program), onBindError is called once and the live ping feature is
// simply unavailable for this run - it is never something startup depends on, and
// ReadStatus's file remains the authoritative record of a shim's last run either
// way.
func StartPingListener(ctx context.Context, onPing func(Ping), onBindError func(error)) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", PingPort))
	if err != nil {
		if onBindError != nil {
			onBindError(err)
		}
		return
	}
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	go acceptPings(ln, onPing)
}

func acceptPings(ln net.Listener, onPing func(Ping)) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return // the listener was closed (ctx cancelled) - nothing more to do
		}
		go handlePing(conn, onPing)
	}
}

func handlePing(conn net.Conn, onPing func(Ping)) {
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	var p Ping
	if err := json.NewDecoder(conn).Decode(&p); err != nil {
		return // a malformed or partial message - not worth reporting, just drop it
	}
	if onPing != nil {
		onPing(p)
	}
}
