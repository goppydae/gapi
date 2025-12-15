package main

import (
	"fmt"
	"time"

	adk "github.com/goppydae/gapi/adk/go"
)

func main() {
	fmt.Println("Event emitter starting...")
	adk.Initialize("event_emitter", "1.0.0", "service")

	// Simple loop to keep alive and emit events if needed
	go func() {
		for {
			time.Sleep(1 * time.Second)
			adk.SendEvent(`{"event": "ping", "data": "hello from go"}`)
		}
	}()

	// Wait for shutdown
	select {}
}
