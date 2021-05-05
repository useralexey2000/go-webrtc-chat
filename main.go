package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

var (
	host                    = "localhost"
	port                    = ":8080"
	tmpl *template.Template = template.Must(template.ParseGlob("templates/*"))
)

func main() {
	mux := http.NewServeMux()
	httpServer := &http.Server{
		Addr:    host + port,
		Handler: mux,
	}
	hub := NewHub()
	quit := make(chan struct{})
	hdone := hub.Run(quit)

	// Sig chan to terminate prog.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	done := shutdown(sigs, httpServer, quit, hdone)

	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.HandleFunc("/", indexHandler(hub))
	mux.HandleFunc("/room", roomHandler)
	mux.HandleFunc("/ws", wsHandler(hub))

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
			// log.Fatalf("Could not gracefully shutdown the server: %v\n", err)
			fmt.Printf("Could not gracefully shutdown the server: %v\n", err)
		}
		defer func() {
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

// HUb .
type Hub struct {
	Rooms     map[string][]*Client
	Reg       chan *Client
	UnReg     chan *Client
	Broadcast chan Message
}

func NewHub() *Hub {
	return &Hub{
		Rooms:     make(map[string][]*Client),
		Reg:       make(chan *Client),
		UnReg:     make(chan *Client),
		Broadcast: make(chan Message),
	}
}

func (h *Hub) Run(quit <-chan struct{}) <-chan struct{} {
	fmt.Println("Hub started")
	done := make(chan struct{})
	go func() {
		for {
			select {
			case c := <-h.Reg:
				fmt.Println("client reg: ", c.ID, c.RoomID)
				h.addClient(c)
			case c := <-h.UnReg:
				fmt.Println("client unreg: ", c.ID, c.RoomID)
				h.remClient(c)
			case msg := <-h.Broadcast:
				// TODO change msg.Data to struct key:val
				fmt.Println("received msg: ", msg.ClientID, msg.RoomID, msg.To, mapKey(msg.Data))
				// if to is not epecified broadcast to all except itself
				if msg.To == "" {
					for _, c := range h.Rooms[msg.RoomID] {
						if c.ID != msg.ClientID {
							c.Ch <- msg
						}
					}
				} else {
					// send to specific user
					for _, c := range h.Rooms[msg.RoomID] {
						if c.ID == msg.To {
							c.Ch <- msg
							break
						}
					}
				}
			case <-quit:
				fmt.Println("quiting hub server")
				for _, r := range h.Rooms {
					for _, c := range r {
						close(c.Ch)
						// c.Conn.Close()
					}
				}
				fmt.Println("All client's channels are closed, returning")
				close(done)
				return
			}
		}
	}()
	return done
}

func (h *Hub) addClient(c *Client) {
	fmt.Println("Adding client: ", c.ID)
	h.Rooms[c.RoomID] = append(h.Rooms[c.RoomID], c)
}

func (h *Hub) remClient(c *Client) {
	fmt.Println("Removing client: ", c.ID)
	for i, cl := range h.Rooms[c.RoomID] {
		if c == cl {
			// c.Conn.Close()
			close(c.Ch)
			h.Rooms[c.RoomID] = append(h.Rooms[c.RoomID][:i], h.Rooms[c.RoomID][i+1:]...)
		}
	}
}

// Client .
type Client struct {
	ID     string
	RoomID string
	Ch     chan Message
	Hb     *Hub
	Conn   *websocket.Conn
	// Done   chan struct{}
}

func (c *Client) Read() {
	defer c.Conn.Close()
	for {
		var msg Message
		if err := c.Conn.ReadJSON(&msg); err != nil {
			fmt.Printf("cant read from conn %s, error: %v\n", c.ID, err)
			c.Hb.UnReg <- c
			return
		}
		msg.ClientID = c.ID
		msg.RoomID = c.RoomID
		c.Hb.Broadcast <- msg
	}

}
func (c *Client) Write() {
	defer func() {
		fmt.Println("Closing connection of client ", c.ID)
		c.Conn.Close()
	}()
	for {
		msg, ok := <-c.Ch
		// Channel is closed
		if !ok {
			fmt.Printf("Channel of client %s is closed, sending close connection message\n", c.ID)
			c.Conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(1001, "server shutdown"))
			break
		}
		if err := c.Conn.WriteJSON(msg); err != nil {
			fmt.Println("cant write to con: ", err)
			c.Hb.UnReg <- c
			return
		}
	}
}

// Message .
type Message struct {
	ClientID string
	// Message addressed to specific client ID
	To     string
	RoomID string
	Data   map[string]interface{}
}

func indexHandler(h *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			err := tmpl.ExecuteTemplate(w, "index.html", nil)
			if err != nil {
				fmt.Println("cant execute template ", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			return
			// TODO change to directly send to ws
		} else if r.Method == "POST" {
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			uname := r.FormValue("username")
			// Give client random name if not specified
			if uname == "" {
				uname = fmt.Sprint(
					rand.New(rand.NewSource(time.Now().UnixNano())).Int())
			}
			roomID := r.FormValue("roomid")
			fmt.Println("Client requested room: ", roomID, uname)
			http.Redirect(w, r, "/room?id="+roomID+"&username="+uname, http.StatusFound)
			return
		}
		http.Error(w, "Unimplemented method", http.StatusNotImplemented)
	}
}

func roomHandler(w http.ResponseWriter, r *http.Request) {
	clientID := r.FormValue("username")
	roomID := r.FormValue("id")
	err := tmpl.ExecuteTemplate(w, "room.html", Message{ClientID: clientID, RoomID: roomID})
	if err != nil {
		fmt.Println("cant execute template ", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func wsHandler(h *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roomID := r.FormValue("roomid")
		ID := r.FormValue("username")
		fmt.Println("client username and roomid: ", ID, roomID)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			fmt.Println("cant upgrade conn: ", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		client := &Client{
			ID:     ID,
			RoomID: roomID,
			Ch:     make(chan Message),
			Hb:     h,
			Conn:   conn,
			// Done:   make(chan struct{}),
		}
		h.Reg <- client
		go client.Read()
		go client.Write()
	}
}

func mapKey(m map[string]interface{}) string {
	for k := range m {
		return k
	}
	return ""
}
