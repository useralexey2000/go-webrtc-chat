package main

import (
	"context"
	"fmt"
	"go-webrtc-chat/internal/videochat"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var (
	host = "localhost"
	port = ":8080"
)

func main() {
	mux := http.NewServeMux()
	httpServer := &http.Server{
		Addr:    host + port,
		Handler: mux,
	}
	hub := videochat.NewHub()
	quit := make(chan struct{})
	hdone := hub.Run(quit)

	// Sig chan to terminate prog.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	done := shutdown(sigs, httpServer, quit, hdone)

	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("../../web/static"))))
	mux.HandleFunc("/", videochat.IndexHandler(hub))
	mux.HandleFunc("/room", videochat.RoomHandler)
	mux.HandleFunc("/ws", videochat.WsHandler(hub))

	fmt.Printf("http server ready to start: %s%s\n", host, port)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Could not listen on %s: %v\n", host+port, err)
	}

	<-done
	fmt.Println("Exiting")
}

func shutdown(sig chan os.Signal, s *http.Server, quit chan struct{}, hdone <-chan struct{}) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		sigmsg := <-sig
		fmt.Println("Received signal to quit: ", sigmsg)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		s.SetKeepAlivesEnabled(false)
		// shutdown dosent wait ws connections
		if err := s.Shutdown(ctx); err != nil {
			fmt.Printf("Could not gracefully shutdown the server: %v\n", err)
			defer os.Exit(1)
		}
		defer func() {
			// Wait 5 sec to close ws connection
			// TODO find better
			<-time.After(5 * time.Second)
			fmt.Println("Canceling shutdown graceful. Force shutdown remaining conns")
			cancel()
			close(done)
		}()
		fmt.Println("Sending quit to hub to cleaup")
		close(quit)
		<-hdone
		fmt.Println("Received done from hub")
	}()
	return done
}
