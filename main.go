package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"runtime"

	"github.com/coreos/go-oidc"
	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/oauth2"
	"pixel-galaxy-client/game"
)

var (
	clientID     = "myClient"
	clientSecret = "your-client-secret"
	redirectURL  = "http://localhost:8082/callback"
	keycloakURL  = "http://localhost:8080/realms/pixel-galaxy"
	config       oauth2.Config
	provider     *oidc.Provider
	loginChan    = make(chan string)
)

func main() {
	// Initialize OAuth2 and OIDC provider
	ctx := context.Background()
	var err error
	provider, err = oidc.NewProvider(ctx, keycloakURL)
	if err != nil {
		log.Fatalf("Failed to get provider: %v", err)
	}

	config = oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  redirectURL,
		Scopes:       []string{oidc.ScopeOpenID, "profile"},
	}

	// Redirect user to login
	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		log.Println("Redirecting to Keycloak login")
		http.Redirect(w, r, config.AuthCodeURL("state"), http.StatusFound)
	})

	// Handle callback from Keycloak
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		log.Println("Received callback from Keycloak")
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}

		code := r.FormValue("code")
		oauth2Token, err := config.Exchange(ctx, code)
		if err != nil {
			http.Error(w, "Failed to exchange token", http.StatusInternalServerError)
			log.Printf("Exchange error: %v", err)
			return
		}

		rawIDToken, ok := oauth2Token.Extra("id_token").(string)
		if !ok {
			http.Error(w, "No id_token field in oauth2 token", http.StatusInternalServerError)
			return
		}

		idToken, err := provider.Verifier(&oidc.Config{ClientID: clientID}).Verify(ctx, rawIDToken)
		if err != nil {
			http.Error(w, "Failed to verify ID Token", http.StatusInternalServerError)
			log.Printf("Verification error: %v", err)
			return
		}

		var claims struct {
			Username string `json:"preferred_username"`
		}
		if err := idToken.Claims(&claims); err != nil {
			http.Error(w, "Failed to parse claims", http.StatusInternalServerError)
			return
		}

		// Print the username
		fmt.Fprintf(w, "Hello, %s! You can now close this browser window and return to the game.", claims.Username)
		log.Printf("Username: %s", claims.Username)

		// Signal successful login
		loginChan <- claims.Username
	})

	// Start the HTTP server
	go func() {
		log.Println("Starting server at http://localhost:8082")
		log.Fatal(http.ListenAndServe(":8082", nil))
	}()

	// Open the user's browser to initiate the login process
	openBrowser("http://localhost:8082/login")

	// Wait for the login process to complete
	username := <-loginChan
	log.Printf("Logged in as: %s", username)

	// Close the browser (guidance)
	fmt.Println("Please close your browser and return to the game.")

	// The rest of your game initialization
	runGame(username)
}

// runGame initializes and runs the game
func runGame(username string) {
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
		Username:     username, // Set the username here
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

	ebiten.SetWindowSize(800, 600) // Default window size
	ebiten.SetWindowTitle("MMO Client")
	ebiten.SetWindowResizable(true)
	ebiten.SetRunnableOnUnfocused(true) // Allow the game to run when the window is not focused
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}

// openBrowser opens the specified URL in the default browser
func openBrowser(url string) {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
		args = []string{url}
	}

	if err := exec.Command(cmd, args...).Start(); err != nil {
		log.Fatal(err)
	}
}
