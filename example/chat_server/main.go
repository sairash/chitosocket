package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"

	"github.com/sairash/chitosocket"
)

func main() {
	numberOfCpu := 1
	runtime.GOMAXPROCS(numberOfCpu)
	socket, err := chitosocket.StartUp(numberOfCpu)
	if err != nil {
		log.Fatalf("Failed to create socket: %v", err)
	}
	defer socket.Close()

	socket.On["connected"] = func(sub *chitosocket.Subscriber, data []byte) {
		welcome := map[string]interface{}{
			"id": sub.ID,
		}
		welcomeData, _ := json.Marshal(welcome)
		socket.EmitDirect(sub, "welcome", welcomeData)
	}

	socket.On["join"] = func(sub *chitosocket.Subscriber, data []byte) {
		var req struct {
			Room string `json:"room"`
		}
		if err := json.Unmarshal(data, &req); err != nil || req.Room == "" {
			return
		}
		socket.AddToRoom(sub, req.Room)

		notification := map[string]string{
			"room": req.Room,
			"user": sub.ID,
		}
		notifyData, _ := json.Marshal(notification)
		socket.Emit("user_joined", notifyData, req.Room)
	}

	socket.On["leave"] = func(sub *chitosocket.Subscriber, data []byte) {
		var req struct {
			Room string `json:"room"`
		}
		if err := json.Unmarshal(data, &req); err != nil || req.Room == "" {
			return
		}

		notification := map[string]string{
			"room": req.Room,
			"user": sub.ID,
		}
		notifyData, _ := json.Marshal(notification)
		socket.Emit("user_left", notifyData, req.Room)

		socket.RemoveFromRoom(sub, req.Room)
	}

	socket.On["chat"] = func(sub *chitosocket.Subscriber, data []byte) {
		var chatMsg struct {
			Room string `json:"room"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(data, &chatMsg); err != nil {
			return
		}

		response := map[string]string{
			"room":   chatMsg.Room,
			"sender": sub.ID,
			"text":   chatMsg.Text,
		}
		responseData, _ := json.Marshal(response)

		members := socket.GetRoomMembers(chatMsg.Room)
		for _, member := range members {
			if member.ID != sub.ID {
				socket.EmitDirect(member, "chat", responseData)
			}
		}
	}

	socket.On["broadcast"] = func(sub *chitosocket.Subscriber, data []byte) {
		socket.Broadcast("announcement", data)
	}

	socket.On["echo"] = func(sub *chitosocket.Subscriber, data []byte) {
		socket.EmitDirect(sub, "echo", data)
	}

	socket.On["ping"] = func(sub *chitosocket.Subscriber, data []byte) {
		socket.EmitDirect(sub, "pong", data)
	}
	socket.On["disconnect"] = func(sub *chitosocket.Subscriber, data []byte) {}

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		_, _, sub, err := socket.UpgradeConnection(r, w)
		if err != nil {
			return
		}
		if handler, ok := socket.On["connected"]; ok {
			handler(sub, nil)
		}
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})

	fmt.Fprintf(os.Stderr, "ChitoSocket server starting on :8080\n")
	fmt.Fprintf(os.Stderr, "CPU cores: %d\n", numberOfCpu)
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
