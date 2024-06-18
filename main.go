package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"log"
	"pixel-galaxy-client/game"
)

func main() {
	playerID := "PLAYER_ID"

	conn, err := game.ConnectToServer(playerID)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer conn.Close()

	clientID := playerID
	log.Printf("Client ID: %s", clientID)

	g := &game.Game{
		Conn:         conn,
		Players:      make(map[string]game.Player),
		ClientID:     clientID,
		PlayerImages: make(map[game.Direction]map[game.AnimationType][]*ebiten.Image),
		AnimationSpeeds: map[game.AnimationType]int{
			game.Idle: 30, // Idle animation speed
			game.Walk: 10, // Walk animation speed
		},
	}

	err = g.LoadFrames("assets/sprites/player/animations")
	if err != nil {
		log.Fatal(err)
	}

	go g.ListenToServer()
	go g.PrintCoordinates()

	ebiten.SetWindowSize(game.ScreenWidth, game.ScreenHeight)
	ebiten.SetWindowTitle("MMO Client")
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
