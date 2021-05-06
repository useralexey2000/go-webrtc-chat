package videochat

import (
	"fmt"
	"html/template"
	"math/rand"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var (
	tmpl *template.Template = template.Must(template.ParseGlob("../../web/templates/*"))

	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

func IndexHandler(h *Hub) http.HandlerFunc {
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

func RoomHandler(w http.ResponseWriter, r *http.Request) {
	clientID := r.FormValue("username")
	roomID := r.FormValue("id")
	err := tmpl.ExecuteTemplate(w, "room.html", Message{ClientID: clientID, RoomID: roomID})
	if err != nil {
		fmt.Println("cant execute template ", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func WsHandler(h *Hub) http.HandlerFunc {
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
