package game

import (
	"fmt"
	"github.com/gorilla/websocket"
	"image/color"
	"log"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

const (
	ScreenWidth   = 800
	ScreenHeight  = 600
	debounceTime  = 200 * time.Millisecond // Debounce time for key press
	maxChatLines  = 5                      // Maximum number of chat lines to display
	maxMessageLen = 100                    // Maximum length of a chat message
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
	PlayerImages       map[Direction]map[AnimationType][]*ebiten.Image
	Conn               *websocket.Conn
	Players            map[string]Player
	ClientID           string
	FrameIndex         int
	AnimationTick      int
	AnimationSpeed     int
	AnimationSpeeds    map[AnimationType]int // Animation speeds for different actions
	mu                 sync.Mutex            // Mutex to protect shared assets
	Direction          Direction
	IsMoving           bool
	IsMapEditorActive  bool      // State for map editor
	IsInventoryActive  bool      // State for inventory UI
	IsChatBoxActive    bool      // State for chat box UI
	lastToggleTime     time.Time // Time of the last toggle
	chatMessages       []string  // Chat messages
	currentChatMessage string    // Current chat message being typed
}

type Player struct {
	ID string  `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
}

func (g *Game) Update() error {
	g.IsMoving = false

	// Handle key presses for toggling UI elements with debouncing
	now := time.Now()
	if ebiten.IsKeyPressed(ebiten.KeyM) && now.Sub(g.lastToggleTime) > debounceTime {
		g.IsMapEditorActive = !g.IsMapEditorActive
		g.lastToggleTime = now
	}
	if ebiten.IsKeyPressed(ebiten.KeyI) && now.Sub(g.lastToggleTime) > debounceTime {
		g.IsInventoryActive = !g.IsInventoryActive
		g.lastToggleTime = now
	}
	if ebiten.IsKeyPressed(ebiten.KeyTab) && now.Sub(g.lastToggleTime) > debounceTime {
		g.IsChatBoxActive = !g.IsChatBoxActive
		g.lastToggleTime = now
	}

	// If the chatbox is active, handle text input
	if g.IsChatBoxActive {
		g.handleChatInput()
		return nil
	}

	// If any other UI is active, skip regular game updates
	if g.IsMapEditorActive || g.IsInventoryActive {
		return nil
	}

	upPressed := ebiten.IsKeyPressed(ebiten.KeyUp)
	downPressed := ebiten.IsKeyPressed(ebiten.KeyDown)
	leftPressed := ebiten.IsKeyPressed(ebiten.KeyLeft)
	rightPressed := ebiten.IsKeyPressed(ebiten.KeyRight)

	if upPressed {
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

func (g *Game) handleChatInput() {
	// Handle text input for chat
	for _, char := range ebiten.InputChars() {
		if len(g.currentChatMessage) < maxMessageLen {
			g.currentChatMessage += string(char)
		}
	}

	// Handle backspace
	if ebiten.IsKeyPressed(ebiten.KeyBackspace) && len(g.currentChatMessage) > 0 {
		g.currentChatMessage = g.currentChatMessage[:len(g.currentChatMessage)-1]
	}

	// Handle enter key to send the chat message
	if ebiten.IsKeyPressed(ebiten.KeyEnter) {
		if len(g.currentChatMessage) > 0 {
			g.chatMessages = append(g.chatMessages, g.currentChatMessage)
			if len(g.chatMessages) > maxChatLines {
				g.chatMessages = g.chatMessages[1:] // Remove the oldest message if we exceed the max chat lines
			}
			g.currentChatMessage = ""
		}
	}
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

	if g.IsMapEditorActive {
		g.drawMapEditor(screen)
		return
	}

	if g.IsInventoryActive {
		g.drawInventory(screen)
		return
	}

	if g.IsChatBoxActive {
		g.drawChatBox(screen)
	}

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

func (g *Game) drawMapEditor(screen *ebiten.Image) {
	screen.Fill(color.RGBA{255, 255, 255, 255}) // White background for the map editor
	ebitenutil.DebugPrintAt(screen, "Map Editor Mode", 10, 10)
	// Additional UI elements for the map editor can be drawn here
}

func (g *Game) drawInventory(screen *ebiten.Image) {
	screen.Fill(color.RGBA{200, 200, 200, 255}) // Light grey background for the inventory
	ebitenutil.DebugPrintAt(screen, "Inventory", 10, 10)
	// Additional UI elements for the inventory can be drawn here
}

func (g *Game) drawChatBox(screen *ebiten.Image) {
	// Semi-transparent background for the chat box
	chatBoxHeight := 100.0
	chatBoxColor := color.RGBA{50, 50, 50, 150}
	ebitenutil.DrawRect(screen, 0, ScreenHeight-chatBoxHeight, ScreenWidth, chatBoxHeight, chatBoxColor)

	// Display chat messages
	for i, msg := range g.chatMessages {
		ebitenutil.DebugPrintAt(screen, msg, 10, int(ScreenHeight-chatBoxHeight)+10+i*20)
	}

	// Display current chat message being typed
	ebitenutil.DebugPrintAt(screen, "> "+g.currentChatMessage, 10, int(ScreenHeight-chatBoxHeight)+10+len(g.chatMessages)*20)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int) {
	return ScreenWidth, ScreenHeight
}
