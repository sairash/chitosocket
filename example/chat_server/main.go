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
		socket.Emit("user_joined", sub, notifyData, req.Room)
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
		socket.Emit("user_left", sub, notifyData, req.Room)

		socket.RemoveFromRoom(sub, req.Room)
	}

	socket.On["chat"] = func(sub *chitosocket.Subscriber, data []byte) {
		var chatMsg struct {
			Room string `json:"room"`
			Text string `json:"text"`
		}
		fmt.Println("chat", chatMsg.Room, chatMsg.Text)
		if err := json.Unmarshal(data, &chatMsg); err != nil {
			fmt.Println("chat error", err)
			return
		}
		fmt.Println("chat", chatMsg.Room, chatMsg.Text)

		response := map[string]string{
			"room":   chatMsg.Room,
			"sender": sub.ID,
			"text":   chatMsg.Text,
		}

		responseData, _ := json.Marshal(response)
		fmt.Println("response", chatMsg.Room, string(responseData))
		socket.Emit("chat", sub, responseData, chatMsg.Room)
	}

	socket.On["broadcast"] = func(sub *chitosocket.Subscriber, data []byte) {
		socket.Broadcast("announcement", data)
	}

	socket.On["echo"] = func(sub *chitosocket.Subscriber, data []byte) {
		socket.EmitDirect(sub, "echo", data)
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
		http.ServeFile(w, r, "example/chat_server/index.html")
	})

	http.HandleFunc("/chitosocket.js", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "example/chat_server/chitosocket.js")
	})

	fmt.Fprintf(os.Stderr, "ChitoSocket server starting on :8080\n")
	fmt.Fprintf(os.Stderr, "CPU cores: %d\n", numberOfCpu)
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
