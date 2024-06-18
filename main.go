package main

import (
	"fmt"
	"image/color"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

const (
	ScreenWidth  = 800
	ScreenHeight = 600
)

type Direction int

const (
	North Direction = iota
	East
	South
	West
)

type AnimationType int

const (
	Idle AnimationType = iota
	Walk
)

type Game struct {
	playerImages   map[Direction]map[AnimationType][]*ebiten.Image
	conn           *websocket.Conn
	players        map[string]Player
	clientID       string
	frameIndex     int
	animationTick  int
	animationSpeed int
	mu             sync.Mutex // Mutex to protect shared assets
	direction      Direction
	isMoving       bool
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
	g.isMoving = false

	if ebiten.IsKeyPressed(ebiten.KeyUp) {
		g.conn.WriteJSON(Input{Type: "move_up", Amount: 2})
		g.setDirection(North)
		g.isMoving = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyDown) {
		g.conn.WriteJSON(Input{Type: "move_down", Amount: 2})
		g.setDirection(South)
		g.isMoving = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyLeft) {
		g.conn.WriteJSON(Input{Type: "move_left", Amount: 2})
		g.setDirection(West)
		g.isMoving = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyRight) {
		g.conn.WriteJSON(Input{Type: "move_right", Amount: 2})
		g.setDirection(East)
		g.isMoving = true
	}

	// Increment animation tick and update frame index based on animation speed
	g.animationTick++
	if g.animationTick >= g.animationSpeed {
		g.animationTick = 0
		g.updateFrameIndex()
	}

	return nil
}

func (g *Game) setDirection(direction Direction) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.direction != direction {
		g.direction = direction
		g.frameIndex = 0
	}
}

func (g *Game) updateFrameIndex() {
	g.mu.Lock()
	defer g.mu.Unlock()
	animationType := Idle
	if g.isMoving {
		animationType = Walk
	}
	frameCount := len(g.playerImages[g.direction][animationType])
	if frameCount > 0 {
		g.frameIndex = (g.frameIndex + 1) % frameCount
	} else {
		g.frameIndex = 0
	}
}

func (g *Game) getCurrentFrame() (*ebiten.Image, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	animationType := Idle
	if g.isMoving {
		animationType = Walk
	}
	frames := g.playerImages[g.direction][animationType]
	if len(frames) > 0 {
		if g.frameIndex < len(frames) {
			return frames[g.frameIndex], nil
		}
		return nil, fmt.Errorf("frame index out of range")
	}
	return nil, fmt.Errorf("no frames available")
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{0, 0, 0, 255})

	frame, err := g.getCurrentFrame()
	if err != nil {
		log.Println("Error getting current frame:", err)
	} else if frame != nil {
		g.mu.Lock()
		defer g.mu.Unlock()
		for _, player := range g.players {
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(player.X, player.Y)
			screen.DrawImage(frame, op)
		}
	}

	// Display coordinates in the top-left corner for the current client
	if player, ok := g.players[g.clientID]; ok {
		coords := fmt.Sprintf("X: %.2f, Y: %.2f", player.X, player.Y)
		ebitenutil.DebugPrintAt(screen, coords, 10, 10)
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int) {
	return ScreenWidth, ScreenHeight
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

func loadFrames(dir string) ([]*ebiten.Image, error) {
	var frames []*ebiten.Image

	// Read all files in the directory
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	// Sort files to ensure they are in numerical order
	var frameFiles []string
	for _, file := range files {
		if !file.IsDir() {
			frameFiles = append(frameFiles, file.Name())
		}
	}
	sort.Strings(frameFiles)

	// Load each frame
	for _, file := range frameFiles {
		img, _, err := ebitenutil.NewImageFromFile(filepath.Join(dir, file))
		if err != nil {
			return nil, err
		}
		frames = append(frames, img)
	}

	return frames, nil
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
		conn:           conn,
		players:        make(map[string]Player),
		clientID:       clientID,
		animationSpeed: 20, // Set animation speed (higher value means slower animation)
		playerImages:   make(map[Direction]map[AnimationType][]*ebiten.Image),
	}

	// Load all frames for each direction and animation type
	directions := map[Direction]string{
		North: "north",
		East:  "east",
		South: "south",
		West:  "west",
	}

	animationTypes := map[AnimationType]string{
		Idle: "idle",
		Walk: "walk",
	}

	for direction, dirName := range directions {
		game.playerImages[direction] = make(map[AnimationType][]*ebiten.Image)
		for animType, animName := range animationTypes {
			dirPath := filepath.Join("assets/sprites/player/base", animName, dirName)
			frames, err := loadFrames(dirPath)
			if err != nil {
				log.Fatal(err)
			}
			game.playerImages[direction][animType] = frames
		}
	}

	go game.listenToServer()
	go game.printCoordinates()

	ebiten.SetWindowSize(ScreenWidth, ScreenHeight)
	ebiten.SetWindowTitle("MMO Client")
	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}
