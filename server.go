package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// Configuration structures
type Config struct {
	Authentication AuthenticationConfig `json:"authentication"`
	Server         ServerConfig         `json:"server"`
}

type AuthenticationConfig struct {
	Enabled  bool   `json:"enabled"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type ServerConfig struct {
	AllowExternalAccess bool `json:"allow_external_access"`
}

var appConfig Config

// Data structures
type User struct {
	Username string `json:"username"`
}

type ProcessingOptions struct {
	Crop  bool `json:"crop"`
	Round bool `json:"round"`
}

type SubmitRequest struct {
	Username  string            `json:"username"`
	ImageData string            `json:"imageData"` // base64 encoded
	Options   ProcessingOptions `json:"options"`
}

type PreviewRequest struct {
	ImageData string            `json:"imageData"` // base64 encoded
	Options   ProcessingOptions `json:"options"`
}

func main() {
	// Load configuration
	loadConfig()

	// Setup routes
	http.HandleFunc("/", authMiddleware(serveHTML))
	http.HandleFunc("/api/users", authMiddleware(listUsers))
	http.HandleFunc("/api/user-photo", authMiddleware(getUserPhoto))
	http.HandleFunc("/api/submit", authMiddleware(submitProfilePicture))
	http.HandleFunc("/api/preview", authMiddleware(previewImage))

	// Determine listen address
	addr := "localhost:8080"
	if appConfig.Authentication.Enabled {
		if appConfig.Server.AllowExternalAccess {
			addr = ":8080"
			fmt.Println("WARNING: External access enabled. Server listening on all interfaces.")
		}
	} else {
		if appConfig.Server.AllowExternalAccess {
			fmt.Println("NOTICE: External access disabled because authentication is not enabled.")
		}
	}

	fmt.Printf("Server starting on http://%s\n", addr)
	// Open browser instruction
	if !appConfig.Server.AllowExternalAccess || !appConfig.Authentication.Enabled {
		fmt.Println("Access the interface at http://localhost:8080")
	}

	log.Fatal(http.ListenAndServe(addr, nil))
}

func loadConfig() {
	file, err := os.Open("config.json")
	if err != nil {
		log.Printf("Could not open config.json, using defaults: %v", err)
		appConfig = Config{
			Authentication: AuthenticationConfig{Enabled: false},
			Server:         ServerConfig{AllowExternalAccess: false},
		}
		return
	}
	defer file.Close()

	if err := json.NewDecoder(file).Decode(&appConfig); err != nil {
		log.Fatal("Error parsing config.json:", err)
	}
}

// Middleware for Basic Auth
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if appConfig.Authentication.Enabled {
			user, pass, ok := r.BasicAuth()
			if !ok || user != appConfig.Authentication.Username || pass != appConfig.Authentication.Password {
				w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

func serveHTML(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func listUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// PowerShell command to get Domain Users
	psCommand := `Get-ADGroupMember -Identity "Domain Users" | Where-Object {$_.objectClass -eq "user"} | Get-ADUser | Select-Object -ExpandProperty SamAccountName | Sort-Object`

	cmd := exec.Command("powershell", "-NoProfile", "-Command", psCommand)
	output, err := cmd.CombinedOutput()

	if err != nil {
		log.Printf("Error listing users: %v, Output: %s", err, string(output))
		// Fallback for development/testing without AD
		if strings.Contains(err.Error(), "executable file not found") {
			json.NewEncoder(w).Encode([]User{{Username: "TestUser1"}, {Username: "TestUser2"}})
			return
		}
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to list users: %s", string(output)))
		return
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var users []User
	for _, line := range lines {
		username := strings.TrimSpace(line)
		if username != "" {
			users = append(users, User{Username: username})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

func getUserPhoto(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	username := r.URL.Query().Get("username")
	if username == "" {
		writeJSONError(w, http.StatusBadRequest, "Username parameter required")
		return
	}

	// PowerShell command to get user's thumbnailPhoto
	psCommand := fmt.Sprintf(`
		$user = Get-ADUser -Identity '%s' -Properties thumbnailPhoto
		if ($user.thumbnailPhoto) {
			[System.Convert]::ToBase64String($user.thumbnailPhoto)
		}
	`, username)

	cmd := exec.Command("powershell", "-NoProfile", "-Command", psCommand)
	output, err := cmd.CombinedOutput()

	if err != nil {
		log.Printf("Error getting photo for %s: %v, Output: %s", username, err, string(output))
		// Return empty response for users without photos
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"imageData": ""})
		return
	}

	base64Photo := strings.TrimSpace(string(output))
	if base64Photo == "" {
		// No photo set
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"imageData": ""})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"imageData": "data:image/jpeg;base64," + base64Photo,
	})
}

func previewImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req PreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.ImageData == "" {
		writeJSONError(w, http.StatusBadRequest, "Image data required")
		return
	}

	processedBytes, err := processImageFromBase64(req.ImageData, req.Options, false) // false = don't compress hard for preview, just return valid jpeg
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Return base64
	b64 := base64.StdEncoding.EncodeToString(processedBytes)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"imageData": "data:image/jpeg;base64," + b64,
	})
}

func submitProfilePicture(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req SubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Username == "" || req.ImageData == "" {
		writeJSONError(w, http.StatusBadRequest, "Username and image data are required")
		return
	}

	processedBytes, err := processImageFromBase64(req.ImageData, req.Options, true) // true = compress for AD
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to process image: %v", err))
		return
	}

	if err := setADUserPhoto(req.Username, processedBytes); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to set AD photo: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": fmt.Sprintf("Profile picture updated for %s", req.Username),
	})
}

func processImageFromBase64(base64Data string, options ProcessingOptions, optimizeSize bool) ([]byte, error) {
	// Remove header if present
	if idx := strings.Index(base64Data, ","); idx != -1 {
		base64Data = base64Data[idx+1:]
	}

	imgBytes, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return nil, fmt.Errorf("invalid base64")
	}

	img, format, err := image.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %v", err)
	}

	// Validate format (No GIF)
	allowedFormats := map[string]bool{
		"jpeg": true,
		"jpg":  true,
		"png":  true,
	}
	if !allowedFormats[strings.ToLower(format)] {
		return nil, fmt.Errorf("unsupported image format: %s (only JPEG and PNG allowed)", format)
	}

	// 1. Crop to Square (Center) if requested or if Round is requested (Round implies square)
	if options.Crop || options.Round {
		img = cropToSquare(img)
	}

	// 2. Round (Apply Circle Mask)
	if options.Round {
		img = applyCircleMask(img)
	}

	// 3. Compress / Encode
	var buf bytes.Buffer

	// If optimization is needed for AD (max 100KB)
	if optimizeSize {
		quality := 90
		for quality > 10 {
			buf.Reset()
			err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
			if err != nil {
				return nil, fmt.Errorf("failed to encode JPEG: %v", err)
			}
			if buf.Len() <= 100*1024 {
				return buf.Bytes(), nil
			}
			quality -= 10
		}
	} else {
		// Just return a decent quality for preview
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
		if err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func cropToSquare(img image.Image) image.Image {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Calculate new square dimensions
	size := width
	if height < width {
		size = height
	}

	// Calculate center offset
	x0 := (width - size) / 2
	y0 := (height - size) / 2

	// Create a new RGBA image
	dst := image.NewRGBA(image.Rect(0, 0, size, size))

	// Draw the center part of the source image onto the destination
	// The source point (x0 + bounds.Min.X, y0 + bounds.Min.Y) maps to (0,0) in dst
	draw.Draw(dst, dst.Bounds(), img, image.Point{X: x0 + bounds.Min.X, Y: y0 + bounds.Min.Y}, draw.Src)

	return dst
}

func applyCircleMask(img image.Image) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// Ensure we are working with an RGBA image for drawing
	dst := image.NewRGBA(bounds)

	// Draw the original image
	draw.Draw(dst, bounds, img, bounds.Min, draw.Src)

	// Create a mask (circle)
	// We want to make corners white (since JPEG doesn't support transparency)
	// Strategy: Draw original image. Iterate pixels? Too slow in Go/JS?
	// Faster: Create a mask image.
	// Or: Calculate distance from center for each pixel.

	// Center
	cx, cy := float64(w)/2, float64(h)/2
	radius := float64(w) / 2

	// Since we are outputting JPEG, 'transparent' means White for AD usually
	white := color.RGBA{255, 255, 255, 255}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx := float64(x) - cx + 0.5
			dy := float64(y) - cy + 0.5
			distSq := dx*dx + dy*dy

			if distSq > radius*radius {
				dst.Set(x, y, white)
			}
		}
	}

	return dst
}

func setADUserPhoto(username string, photoBytes []byte) error {
	base64Photo := base64.StdEncoding.EncodeToString(photoBytes)

	psCommand := fmt.Sprintf(`
		$base64 = $input | Out-String
		$base64 = $base64.Trim()
		$bytes = [System.Convert]::FromBase64String($base64)
		Set-ADUser -Identity '%s' -Replace @{thumbnailPhoto=$bytes}
	`, username)

	cmd := exec.Command("powershell", "-NoProfile", "-Command", psCommand)
	cmd.Stdin = strings.NewReader(base64Photo)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("PowerShell error: %v, Output: %s", err, string(output))
	}

	return nil
}
