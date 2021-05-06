package videochat

import (
	"fmt"

	"github.com/gorilla/websocket"
)

// Client .
type Client struct {
	ID     string
	RoomID string
	Ch     chan Message
	Hb     *Hub
	Conn   *websocket.Conn
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
