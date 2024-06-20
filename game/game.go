package game

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"image/color"
	"log"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font/basicfont"
)

const (
	ScreenWidth         = 800
	ScreenHeight        = 600
	debounceTime        = 200 * time.Millisecond // Debounce time for key press
	maxChatLines        = 10                     // Maximum number of chat lines to display
	maxMessageLen       = 100                    // Maximum length of a chat message
	cursorBlinkInterval = 500 * time.Millisecond // Blink interval for cursor
	scrollBarWidth      = 10                     // Width of the scroll bar
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
	PlayerImages         map[Direction]map[AnimationType][]*ebiten.Image
	Conn                 *websocket.Conn
	Players              map[string]Player
	ClientID             string
	FrameIndex           int
	AnimationTick        int
	AnimationSpeed       int
	AnimationSpeeds      map[AnimationType]int // Animation speeds for different actions
	mu                   sync.Mutex            // Mutex to protect shared assets
	Direction            Direction
	IsMoving             bool
	IsMapEditorActive    bool      // State for map editor
	IsInventoryActive    bool      // State for inventory UI
	IsChatBoxActive      bool      // State for chat box UI
	IsChatCursorActive   bool      // State for chat cursor focus
	lastToggleTime       time.Time // Time of the last toggle
	lastCursorBlink      time.Time // Time of the last cursor blink
	cursorVisible        bool      // Cursor visibility state for blinking
	chatMessages         []string  // Chat messages
	currentChatMessage   string    // Current chat message being typed
	Username             string    // Username of the current user
	chatScrollOffset     int       // Scroll offset for chat messages
	isDraggingScrollBar  bool      // State for scroll bar dragging
	dragStartY           float64   // Y position where the drag started
	originalScrollOffset int       // Scroll offset before dragging started
}

type Player struct {
	ID string  `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json://y"`
}

type ChatMessage struct {
	Username string `json:"username"`
	Message  string `json:"message"`
}

func (g *Game) Update() error {
	g.IsMoving = false

	now := time.Now()
	// Toggle chatbox visibility with Tab key
	if ebiten.IsKeyPressed(ebiten.KeyTab) && now.Sub(g.lastToggleTime) > debounceTime {
		g.IsChatBoxActive = !g.IsChatBoxActive
		g.lastToggleTime = now
		// If chatbox is being closed, also deactivate cursor
		if !g.IsChatBoxActive {
			g.IsChatCursorActive = false
		}
	}

	// Toggle chat cursor focus with Enter key when chatbox is visible
	if g.IsChatBoxActive && ebiten.IsKeyPressed(ebiten.KeyEnter) && now.Sub(g.lastToggleTime) > debounceTime {
		g.IsChatCursorActive = !g.IsChatCursorActive
		g.lastToggleTime = now
		// If chat cursor is deactivated, handle the chat message if there is one
		if !g.IsChatCursorActive && len(g.currentChatMessage) > 0 {
			chatMessage := ChatMessage{
				Username: g.Username,
				Message:  g.currentChatMessage,
			}
			log.Printf("Sending chat message: %s: %s", chatMessage.Username, chatMessage.Message)
			g.Conn.WriteJSON(chatMessage)
			g.chatMessages = append(g.chatMessages, fmt.Sprintf("%s: %s", chatMessage.Username, chatMessage.Message))
			g.currentChatMessage = ""
			// Scroll to the bottom when a new message is added
			if len(g.chatMessages) > maxChatLines {
				g.chatScrollOffset = len(g.chatMessages) - maxChatLines
			} else {
				g.chatScrollOffset = 0
			}
		}
	}

	// If chat cursor is active, handle chat input and cursor blinking
	if g.IsChatCursorActive {
		g.handleChatInput()
		// Handle cursor blinking
		if now.Sub(g.lastCursorBlink) >= cursorBlinkInterval {
			g.cursorVisible = !g.cursorVisible
			g.lastCursorBlink = now
		}
	}

	// Handle key presses for toggling other UI elements with debouncing
	if !g.IsChatCursorActive {
		if ebiten.IsKeyPressed(ebiten.KeyM) && now.Sub(g.lastToggleTime) > debounceTime {
			g.IsMapEditorActive = !g.IsMapEditorActive
			g.lastToggleTime = now
		}
		if ebiten.IsKeyPressed(ebiten.KeyI) && now.Sub(g.lastToggleTime) > debounceTime {
			g.IsInventoryActive = !g.IsInventoryActive
			g.lastToggleTime = now
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
	}

	// Handle chat scrolling
	if g.IsChatBoxActive {
		// Handle scroll bar dragging
		if g.isDraggingScrollBar {
			if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
				_, mouseY := ebiten.CursorPosition()
				deltaY := float64(mouseY) - g.dragStartY
				chatBoxHeight := 300.0 // Updated to fit 10 lines
				maxScroll := float64(len(g.chatMessages) - maxChatLines)
				newOffset := g.originalScrollOffset + int(deltaY*maxScroll/chatBoxHeight)
				g.chatScrollOffset = clamp(newOffset, 0, len(g.chatMessages)-maxChatLines)
			} else {
				g.isDraggingScrollBar = false
			}
		} else if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
			// Start dragging if the mouse is over the scroll bar
			mouseX, mouseY := ebiten.CursorPosition()
			chatBoxHeight := 300.0 // Updated to fit 10 lines
			scrollBarHeight := chatBoxHeight * float64(maxChatLines) / float64(len(g.chatMessages))
			scrollBarY := float64(ScreenHeight) - chatBoxHeight + (chatBoxHeight-scrollBarHeight)*float64(g.chatScrollOffset)/float64(len(g.chatMessages)-maxChatLines)
			if float64(mouseX) >= ScreenWidth-scrollBarWidth && float64(mouseY) >= scrollBarY && float64(mouseY) <= scrollBarY+scrollBarHeight {
				g.isDraggingScrollBar = true
				g.dragStartY = float64(mouseY)
				g.originalScrollOffset = g.chatScrollOffset
			}
		}

		if ebiten.IsKeyPressed(ebiten.KeyPageUp) {
			g.chatScrollOffset = max(0, g.chatScrollOffset-1)
		} else if ebiten.IsKeyPressed(ebiten.KeyPageDown) {
			g.chatScrollOffset = min(len(g.chatMessages)-maxChatLines, g.chatScrollOffset+1)
		}
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
	// Black, transparent background for the chat box
	chatBoxHeight := 300.0 // Updated to fit 10 lines
	chatBoxColor := color.RGBA{0, 0, 0, 180}
	ebitenutil.DrawRect(screen, 0, float64(ScreenHeight)-chatBoxHeight, ScreenWidth, chatBoxHeight, chatBoxColor)

	// Display chat messages with padding
	padding := 10
	lineHeight := 20
	for i := 0; i < maxChatLines && i < len(g.chatMessages)-g.chatScrollOffset; i++ {
		msg := g.chatMessages[g.chatScrollOffset+i]
		ebitenutil.DebugPrintAt(screen, msg, padding, int(ScreenHeight-chatBoxHeight)+padding+i*lineHeight)
	}

	// Display current chat message being typed
	ebitenutil.DebugPrintAt(screen, "> "+g.currentChatMessage, padding, int(ScreenHeight-chatBoxHeight)+padding+maxChatLines*lineHeight)

	// Draw the blinking cursor if the chat cursor is active
	if g.IsChatCursorActive && g.cursorVisible {
		textBounds := text.BoundString(basicfont.Face7x13, g.currentChatMessage)
		textWidth := textBounds.Max.X - textBounds.Min.X
		cursorX := float64(padding) + float64(textWidth)
		cursorY := float64(ScreenHeight) - chatBoxHeight + float64(padding) + float64(maxChatLines*lineHeight)
		cursorHeight := float64(basicfont.Face7x13.Metrics().Height.Ceil())
		ebitenutil.DrawRect(screen, cursorX, cursorY, 2, cursorHeight, color.White) // Draw cursor as a thin vertical line
	}

	// Draw the scroll bar
	if len(g.chatMessages) > maxChatLines {
		scrollBarHeight := chatBoxHeight * float64(maxChatLines) / float64(len(g.chatMessages))
		scrollBarY := float64(ScreenHeight) - chatBoxHeight + (chatBoxHeight-scrollBarHeight)*float64(g.chatScrollOffset)/float64(len(g.chatMessages)-maxChatLines)
		ebitenutil.DrawRect(screen, float64(ScreenWidth)-scrollBarWidth, scrollBarY, scrollBarWidth, scrollBarHeight, color.Gray{Y: 0x80})
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int) {
	return ScreenWidth, ScreenHeight
}

func (g *Game) handleServerMessages() {
	for {
		_, message, err := g.Conn.ReadMessage()
		if err != nil {
			log.Println("Error reading message:", err)
			return
		}

		var response map[string]string
		if err := json.Unmarshal(message, &response); err != nil {
			log.Println("Error unmarshalling response:", err)
			continue
		}

		switch response["type"] {
		case "login_success":
			g.Username = response["username"]
			log.Printf("Username set to: %s", g.Username)
			// Handle other message types
		}
	}
}

// Helper functions for min and max
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clamp(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
