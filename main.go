package main

import (
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"image/color"
)

type Game struct {
	playerImage *ebiten.Image
	conn        *websocket.Conn
	players     map[string]Player
	clientID    string
	mu          sync.Mutex // Mutex to protect shared resources
}

type Player struct {
	ID string  `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
}

type Input struct {
	Type   string  `json:"type"`
	Amount float64 `json:"amount"`
}

func (g *Game) Update() error {
	if ebiten.IsKeyPressed(ebiten.KeyUp) {
		g.conn.WriteJSON(Input{Type: "move_up", Amount: 2})
	}
	if ebiten.IsKeyPressed(ebiten.KeyDown) {
		g.conn.WriteJSON(Input{Type: "move_down", Amount: 2})
	}
	if ebiten.IsKeyPressed(ebiten.KeyLeft) {
		g.conn.WriteJSON(Input{Type: "move_left", Amount: 2})
	}
	if ebiten.IsKeyPressed(ebiten.KeyRight) {
		g.conn.WriteJSON(Input{Type: "move_right", Amount: 2})
	}

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{0, 0, 0, 255})

	g.mu.Lock()
	defer g.mu.Unlock()

	for _, player := range g.players {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(player.X, player.Y)
		screen.DrawImage(g.playerImage, op)
	}

	// Display coordinates in the top-left corner for the current client
	if player, ok := g.players[g.clientID]; ok {
		coords := fmt.Sprintf("X: %.2f, Y: %.2f", player.X, player.Y)
		ebitenutil.DebugPrintAt(screen, coords, 10, 10)
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int) {
	return 800, 600
}

func (g *Game) listenToServer() {
	for {
		var players map[string]Player
		err := g.conn.ReadJSON(&players)
		if err != nil {
			log.Println("ReadJSON error:", err)
			return
		}

		g.mu.Lock()
		g.players = players
		g.mu.Unlock()
	}
}

func (g *Game) printCoordinates() {
	for {
		time.Sleep(1 * time.Second)
		g.mu.Lock()
		if player, ok := g.players[g.clientID]; ok {
			fmt.Printf("Current coordinates - X: %.2f, Y: %.2f\n", player.X, player.Y)
		}
		g.mu.Unlock()
	}
}

func connectToServer(playerID string) (*websocket.Conn, error) {
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

func main() {
	playerID := "PLAYER_ID"

	conn, err := connectToServer(playerID)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer conn.Close()

	clientID := playerID
	log.Printf("Client ID: %s", clientID)

	game := &Game{
		conn:     conn,
		players:  make(map[string]Player),
		clientID: clientID,
	}
	game.playerImage, _, err = ebitenutil.NewImageFromFile("resources/sprite.png")
	if err != nil {
		log.Fatal(err)
	}

	go game.listenToServer()
	go game.printCoordinates()

	ebiten.SetWindowSize(800, 600)
	ebiten.SetWindowTitle("MMO Client")
	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}
