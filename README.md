# ChitoSocket
## _Less Memory Footprint Socket io_

*⚠️ Still working on it*

Chito → language: Nepali → *छिटो* →  meaning: *Fast* <br/>
Socket → meaning: *Web Socket*
<br/>

ChitoSocket is a fast, lightweight WebSockets library with room systems for Go. It can handle millions of connections and uses very little RAM. ChitoSocket is easy to use, and you can write a blazingly fast WebSockets application in less than 10 lines of code. 
ChitoSocket is built on top of the [gobwas/ws](https://github.com/gobwas/ws) library. It is also based on the findings of the article "[Million WebSockets and Go](https://www.freecodecamp.org/news/million-websockets-and-go-cc58418460bb/)" by FreeCodeCamp.

## Installation

```bash
go get github.com/sairash/chitosocket
```

## Quick Start

```go
package main

import (
    "encoding/json"
    "log"
    "net/http"

    "github.com/sairash/chitosocket"
)

func main() {
    // Create socket server (pass number of CPU cores)
    socket, err := chitosocket.StartUp(1)
    if err != nil {
        log.Fatal(err)
    }
    defer socket.Close()

    // Handle connection
    socket.On["connected"] = func(sub *chitosocket.Subscriber, data []byte) {
        welcome := map[string]string{"id": sub.ID}
        welcomeData, _ := json.Marshal(welcome)
        socket.EmitDirect(sub, "welcome", welcomeData)
    }

    // Handle messages
    socket.On["chat"] = func(sub *chitosocket.Subscriber, data []byte) {
        socket.Emit("chat", data, "general")
    }

    // Handle disconnect
    socket.On["disconnect"] = func(sub *chitosocket.Subscriber, data []byte) {
        log.Printf("Client %s disconnected", sub.ID)
    }

    // HTTP handler for WebSocket upgrades
    http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
        _, _, sub, err := socket.UpgradeConnection(r, w)
        if err != nil {
            return
        }
        if handler, ok := socket.On["connected"]; ok {
            handler(sub, nil)
        }
    })

    log.Println("Server starting on :8080")
    http.ListenAndServe(":8080", nil)
}
```

## API Reference

### Creating a Socket Server

```go
// With default config based on CPU cores
socket, err := chitosocket.StartUp(numCPU)

// With custom config
config := chitosocket.Config{
    NumWorkers:    8,
    NumShards:     16,
    EventChanSize: 8192,
}
socket, err := chitosocket.StartUpWithConfig(config)
```

### Event Handlers

```go
// Listen for events
socket.On["event_name"] = func(sub *chitosocket.Subscriber, data []byte) {
    // Handle event
}

// Special events
socket.On["connected"]   // Called when client connects
socket.On["disconnect"]  // Called when client disconnects
```

### Sending Messages

```go
// Send to specific subscriber
socket.EmitDirect(sub, "event", data)

// Send to all subscribers in room(s)
socket.Emit("event", data, "room1", "room2")

// Broadcast to all connected clients
socket.Broadcast("event", data)
```

### Room Management

```go
// Add subscriber to room
socket.AddToRoom(sub, "room_name")

// Remove subscriber from room
socket.RemoveFromRoom(sub, "room_name")

// Get all members in a room
members := socket.GetRoomMembers("room_name")

// Get count of members in a room
count := socket.GetRoomCount("room_name")
```

### Subscriber Methods

```go
// Send message (non-blocking, returns false if channel full)
sub.Send(payload)

// Check if subscriber is closed
sub.IsClosed()

// Store metadata
sub.Metadata.Store("key", value)
val, ok := sub.Metadata.Load("key")
```

## Benchmark
Cpu Core: 1 ; 
Active Connections: 201624

TIMESTAMP           | CPU %  | RSS (MB) | VSZ (MB) | STARTED 
--------------------|--------|----------|----------|---------
2025-12-30 11:40:59 |  11.5%  | 1676.64  | 3198.67  |  11:29:59

## Running the Example

```bash
cd example/server
go run main.go
```

Then open `http://localhost:8080` in your browser.

## Running Benchmarks

```bash
cd benchmark
go run Benchmark.go
```

*Example Project*
- https://github.com/sairash/chitosocket-example

<br/>

*✨Links✨*
- https://github.com/gobwas/ws
- https://www.freecodecamp.org/news/million-websockets-and-go-cc58418460bb/
- https://github.com/sairash/chitosocket-example

<br/>

*✨Chitosocket Packages✨*
| Langauge | Package Link |
| ------ | ------ |
| EcmaScript/ JS | https://www.npmjs.com/package/@sairash/chitosocket |
|  | * *New Packages Coming Soon* * |

## License
MIT
