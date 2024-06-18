package game

import (
	"fmt"
	"github.com/gorilla/websocket"
	"image/color"
	"log"
	"sync"

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
	PlayerImages    map[Direction]map[AnimationType][]*ebiten.Image
	Conn            *websocket.Conn
	Players         map[string]Player
	ClientID        string
	FrameIndex      int
	AnimationTick   int
	AnimationSpeed  int
	AnimationSpeeds map[AnimationType]int // Animation speeds for different actions
	mu              sync.Mutex            // Mutex to protect shared assets
	Direction       Direction
	IsMoving        bool
}

type Player struct {
	ID string  `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
}

func (g *Game) Update() error {
	g.IsMoving = false

	upPressed := ebiten.IsKeyPressed(ebiten.KeyUp)
	downPressed := ebiten.IsKeyPressed(ebiten.KeyDown)
	leftPressed := ebiten.IsKeyPressed(ebiten.KeyLeft)
	rightPressed := ebiten.IsKeyPressed(ebiten.KeyRight)

	if upPressed && rightPressed {
		g.Conn.WriteJSON(Input{Type: "move_up", Amount: 2})
		g.Conn.WriteJSON(Input{Type: "move_right", Amount: 2})
		g.setDirection(East)
		g.IsMoving = true
	} else if upPressed && leftPressed {
		g.Conn.WriteJSON(Input{Type: "move_up", Amount: 2})
		g.Conn.WriteJSON(Input{Type: "move_left", Amount: 2})
		g.setDirection(West)
		g.IsMoving = true
	} else if downPressed && rightPressed {
		g.Conn.WriteJSON(Input{Type: "move_down", Amount: 2})
		g.Conn.WriteJSON(Input{Type: "move_right", Amount: 2})
		g.setDirection(East)
		g.IsMoving = true
	} else if downPressed && leftPressed {
		g.Conn.WriteJSON(Input{Type: "move_down", Amount: 2})
		g.Conn.WriteJSON(Input{Type: "move_left", Amount: 2})
		g.setDirection(West)
		g.IsMoving = true
	} else if upPressed {
		g.Conn.WriteJSON(Input{Type: "move_up", Amount: 2})
		g.setDirection(North)
		g.IsMoving = true
	} else if downPressed {
		g.Conn.WriteJSON(Input{Type: "move_down", Amount: 2})
		g.setDirection(South)
		g.IsMoving = true
	} else if leftPressed {
		g.Conn.WriteJSON(Input{Type: "move_left", Amount: 2})
		g.setDirection(West)
		g.IsMoving = true
	} else if rightPressed {
		g.Conn.WriteJSON(Input{Type: "move_right", Amount: 2})
		g.setDirection(East)
		g.IsMoving = true
	}

	// Increment animation tick and update frame index based on animation speed
	g.AnimationTick++
	animationType := Idle
	if g.IsMoving {
		animationType = Walk
	}
	g.AnimationSpeed = g.AnimationSpeeds[animationType]
	if g.AnimationTick >= g.AnimationSpeed {
		g.AnimationTick = 0
		g.updateFrameIndex()
	}

	return nil
}

func (g *Game) setDirection(direction Direction) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.Direction != direction {
		g.Direction = direction
		g.FrameIndex = 0
	}
}

func (g *Game) updateFrameIndex() {
	g.mu.Lock()
	defer g.mu.Unlock()
	animationType := Idle
	if g.IsMoving {
		animationType = Walk
	}
	frameCount := len(g.PlayerImages[g.Direction][animationType])
	if frameCount > 0 {
		g.FrameIndex = (g.FrameIndex + 1) % frameCount
	} else {
		g.FrameIndex = 0
	}
}

func (g *Game) getCurrentFrame() (*ebiten.Image, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	animationType := Idle
	if g.IsMoving {
		animationType = Walk
	}
	frames := g.PlayerImages[g.Direction][animationType]
	if len(frames) > 0 {
		if g.FrameIndex < len(frames) {
			return frames[g.FrameIndex], nil
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
		for _, player := range g.Players {
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(player.X, player.Y)
			screen.DrawImage(frame, op)
		}
	}

	// Display coordinates in the top-left corner for the current client
	if player, ok := g.Players[g.ClientID]; ok {
		coords := fmt.Sprintf("X: %.2f, Y: %.2f", player.X, player.Y)
		ebitenutil.DebugPrintAt(screen, coords, 10, 10)
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int) {
	return ScreenWidth, ScreenHeight
}
