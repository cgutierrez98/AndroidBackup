package gallery

import (
	"fmt"
	"html/template"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/disintegration/imaging"
)

type Generator struct{}

type MediaItem struct {
	OriginalRelPath string
	ThumbRelPath    string
	Name            string
	Date            string
	IsVideo         bool
}

func NewGenerator() *Generator {
	return &Generator{}
}

// Generate scans rootPath and creates an index.html with thumbnails
func (g *Generator) Generate(rootPath string, progressCallback func(current, total int)) (int, error) {
	// 1. Scan for media files
	var media []string
	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			// skip thumbnails folder itself if it exists
			if info.Name() == "thumbnails" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".mp4" {
			media = append(media, path)
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("failed to scan directory: %w", err)
	}

	total := len(media)
	if total == 0 {
		return 0, nil
	}

	// 2. Create thumbnails directory
	thumbDir := filepath.Join(rootPath, "thumbnails")
	if err := os.MkdirAll(thumbDir, 0755); err != nil {
		return 0, fmt.Errorf("failed to create thumbnails dir: %w", err)
	}

	// 3. Process Images (Concurrent)
	items := make([]MediaItem, total)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8) // Limit concurrency (cpu bound)

	processed := 0
	var mu sync.Mutex

	for i, pathStr := range media {
		wg.Add(1)
		go func(idx int, srcPath string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			relPath, _ := filepath.Rel(rootPath, srcPath)
			name := filepath.Base(srcPath)
			isVideo := strings.ToLower(filepath.Ext(srcPath)) == ".mp4"

			// Hash name for thumb to avoid path issues? Or keep structure?
			// Simple flat thumbnails folder with hashed names or just unique logical names needed.
			// Let's use relPath but replace separators to flatten it.
			flatName := strings.ReplaceAll(relPath, string(os.PathSeparator), "_")
			flatName = strings.ReplaceAll(flatName, ":", "")
			if isVideo {
				flatName += ".jpg" // Thumb for video
			}
			thumbName := "thumb_" + flatName
			thumbPath := filepath.Join(thumbDir, thumbName)

			item := MediaItem{
				OriginalRelPath: relPath,
				ThumbRelPath:    "thumbnails/" + thumbName,
				Name:            name,
				IsVideo:         isVideo,
			}

			// Generate Thumbnail if doesn't exist
			if _, err := os.Stat(thumbPath); os.IsNotExist(err) {
				if isVideo {
					// Video thumbnailing is hard without ffmpeg.
					// Placeholder for video?
					// Or just skip thumb generation and use a generic icon in HTML?
					// Let's create a generic placeholder image for now.
					createPlaceholder(thumbPath, "VIDEO")
				} else {
					// Image resize
					img, err := imaging.Open(srcPath)
					if err == nil {
						// Resize to 300x300 fill
						thumb := imaging.Fill(img, 300, 300, imaging.Center, imaging.Lanczos)
						imaging.Save(thumb, thumbPath)
					} else {
						// Failed to open image
						createPlaceholder(thumbPath, "ERR")
					}
				}
			}

			mu.Lock()
			items[idx] = item
			processed++
			if progressCallback != nil {
				progressCallback(processed, total)
			}
			mu.Unlock()

		}(i, pathStr)
	}

	wg.Wait()

	// 4. Generate HTML
	if err := generateHTML(rootPath, items); err != nil {
		return processed, err
	}

	return processed, nil
}

func createPlaceholder(path string, label string) {
	// Create a simple gray image
	rect := image.Rect(0, 0, 300, 300)
	img := image.NewRGBA(rect)
	// (Writing text to image in pure go is verbose, let's just make it gray)
	// Just save it.
	f, _ := os.Create(path)
	defer f.Close()
	jpeg.Encode(f, img, nil)
}

const htmlTemplate = `
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Android Backup Gallery</title>
    <style>
        body { font-family: sans-serif; background: #222; color: #eee; margin: 0; padding: 20px; }
        .gallery { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 15px; }
        .item { background: #333; padding: 10px; border-radius: 8px; text-align: center; }
        .item img { max-width: 100%; height: auto; border-radius: 4px; display: block; margin-bottom: 5px; }
        .item a { color: #88c0d0; text-decoration: none; font-size: 0.9em; word-break: break-all; }
        .video-badge { background: #d08770; color: #222; padding: 2px 5px; border-radius: 4px; font-weight: bold; font-size: 0.8em; }
    </style>
</head>
<body>
    <h1>Backup Gallery <span>{{.Count}} items</span></h1>
    <p>Generated on {{.Date}}</p>
    <div class="gallery">
        {{range .Items}}
        <div class="item">
            <a href="{{.OriginalRelPath}}" target="_blank">
                <img src="{{.ThumbRelPath}}" alt="{{.Name}}" loading="lazy">
                {{.Name}}
            </a>
            {{if .IsVideo}}<span class="video-badge">VIDEO</span>{{end}}
        </div>
        {{end}}
    </div>
</body>
</html>
`

func generateHTML(rootPath string, items []MediaItem) error {
	tmpl, err := template.New("gallery").Parse(htmlTemplate)
	if err != nil {
		return err
	}

	data := struct {
		Items []MediaItem
		Count int
		Date  string
	}{
		Items: items,
		Count: len(items),
		Date:  time.Now().Format("2006-01-02 15:04"),
	}

	f, err := os.Create(filepath.Join(rootPath, "index.html"))
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.Execute(f, data)
}
