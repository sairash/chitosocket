package main

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const (
	serverAddr = "127.0.0.1:8080"
	totalConns = 1000000
	rampUpRate = 1000
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var activeConns int64
	var attemptedConns int64
	var skippedConns int64

	u := url.URL{Scheme: "ws", Host: serverAddr, Path: "/ws"}

	isLocal := strings.HasPrefix(serverAddr, "127.") || strings.HasPrefix(serverAddr, "localhost")
	fmt.Printf("Starting Benchmark to %s (Mode: %s)\n", u.String(), map[bool]string{true: "Local", false: "Remote"}[isLocal])

	dialer := &websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	numIPs := 16
	if !isLocal {
		numIPs = 1
	}

	for i := 1; i <= numIPs; i++ {
		if isLocal {
			localIP := fmt.Sprintf("127.0.0.%d", i)
			localAddr := &net.TCPAddr{IP: net.ParseIP(localIP)}
			dialer.NetDial = func(network, addr string) (net.Conn, error) {
				return (&net.Dialer{LocalAddr: localAddr}).DialContext(ctx, network, addr)
			}
		}

		connsForThisIP := totalConns / numIPs
		for j := 0; j < connsForThisIP; j++ {
			select {
			case <-ctx.Done():
				return
			default:
			}

			go func() {
				atomic.AddInt64(&attemptedConns, 1)
				conn, _, err := dialer.Dial(u.String(), nil)
				if err != nil {
					if strings.Contains(err.Error(), "address already in use") {
						atomic.AddInt64(&skippedConns, 1)
						return
					}
					fmt.Printf("\n[FATAL ERROR] %v\n", err)
					cancel()
					return
				}

				atomic.AddInt64(&activeConns, 1)
				defer func() {
					atomic.AddInt64(&activeConns, -1)
					conn.Close()
				}()

				for {
					if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(time.Second)); err != nil {
						return
					}
					select {
					case <-ctx.Done():
						return
					case <-time.After(30 * time.Second):
					}
				}
			}()

			if attemptedConns%500 == 0 {
				fmt.Printf("\rAttempted: %d | Active: %d | Skipped: %d",
					atomic.LoadInt64(&attemptedConns),
					atomic.LoadInt64(&activeConns),
					atomic.LoadInt64(&skippedConns))
			}
			time.Sleep(time.Second / rampUpRate)
		}
	}

	<-ctx.Done()
	fmt.Printf("\nStopped. Final Active: %d\n", atomic.LoadInt64(&activeConns))
}
