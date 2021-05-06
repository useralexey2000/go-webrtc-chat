package videochat

import "fmt"

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

func mapKey(m map[string]interface{}) string {
	for k := range m {
		return k
	}
	return ""
}
