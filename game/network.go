package game

import (
	"log"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

func ConnectToServer(playerID string) (*websocket.Conn, error) {
	u := url.URL{Scheme: "ws", Host: "localhost:8000", Path: "/ws"}
	q := u.Query()
	q.Set("playerId", playerID)
	u.RawQuery = q.Encode()
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, err
	}
	log.Printf("Connected to server: %s", u.String())
	return conn, nil
}

func (g *Game) ListenToServer() {
	for {
		var players map[string]Player
		err := g.Conn.ReadJSON(&players)
		if err != nil {
			log.Println("ReadJSON error:", err)
			return
		}

		g.mu.Lock()
		g.Players = players
		g.mu.Unlock()
	}
}

func (g *Game) PrintCoordinates() {
	for {
		time.Sleep(1 * time.Second)
		g.mu.Lock()
		if player, ok := g.Players[g.ClientID]; ok {
			log.Printf("Current coordinates - X: %.2f, Y: %.2f\n", player.X, player.Y)
		}
		g.mu.Unlock()
	}
}
