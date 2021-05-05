package main

import (
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var (
	host                    = "localhost"
	port                    = ":8080"
	tmpl *template.Template = template.Must(template.ParseGlob("templates/*"))
)

func main() {
	hub := NewHub()
	hub.Run()
	// Sig chan to terminate prog.
	// sigs := make(chan os.Signal, 1)
	// signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	// go func() {
	// 	sig := <-sigs
	// 	fmt.Println(sig)
	// 	close(done)
	// 	os.Exit(0)
	// }()
	fmt.Printf("server started: %s%s\n", host, port)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	http.HandleFunc("/", indexHandler(hub))
	http.HandleFunc("/room", roomHandler)
	http.HandleFunc("/ws", wsHandler(hub))
	log.Fatal(http.ListenAndServe(":8080", nil))
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

func (h *Hub) Run() chan<- struct{} {
	done := make(chan struct{})
	go func() {
		for {
			select {
			case c := <-h.Reg:
				fmt.Println("user reg: ", c.ID, c.RoomID)
				h.addClient(c)
			case c := <-h.UnReg:
				fmt.Println("user unreg: ", c.ID, c.RoomID)
				h.remClient(c)
			case msg := <-h.Broadcast:
				fmt.Println("received msg: ", msg.ClientID, msg.RoomID, msg.To, msg.Data)
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
			case <-done:
				fmt.Println("quiting server")
				for _, r := range h.Rooms {
					for _, c := range r {
						c.Conn.Close()
					}
				}
				return
			}
		}
	}()
	return done
}

func (h *Hub) addClient(c *Client) {
	h.Rooms[c.RoomID] = append(h.Rooms[c.RoomID], c)
}

func (h *Hub) remClient(c *Client) {
	for i, cl := range h.Rooms[c.RoomID] {
		if c == cl {
			c.Conn.Close()
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
	for {
		var msg Message
		if err := c.Conn.ReadJSON(&msg); err != nil {
			fmt.Println("cant read from conn ", err)
			c.Hb.UnReg <- c
			return
		}
		msg.ClientID = c.ID
		msg.RoomID = c.RoomID
		c.Hb.Broadcast <- msg
	}

}
func (c *Client) Write() {
	for {
		msg := <-c.Ch
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
				fmt.Println(err)
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
		fmt.Println(err)
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
		fmt.Println("username and roomid: ", ID, roomID)
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
