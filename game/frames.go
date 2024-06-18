package game

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

func (g *Game) LoadFrames(baseDir string) error {
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
		g.PlayerImages[direction] = make(map[AnimationType][]*ebiten.Image)
		for animType, animName := range animationTypes {
			dirPath := filepath.Join(baseDir, animName, dirName)
			frames, err := loadFramesFromDir(dirPath)
			if err != nil {
				return err
			}
			g.PlayerImages[direction][animType] = frames
		}
	}
	return nil
}

func loadFramesFromDir(dir string) ([]*ebiten.Image, error) {
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
