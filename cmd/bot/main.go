package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/joho/godotenv"
	"github.com/ledongthuc/pdf"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/net/publicsuffix"
)

const (
	loginURL      = "https://cis.nordakademie.de/login"
	transcriptURL = "https://cis.nordakademie.de/studium/pruefungen/pruefungsergebnisse?tx_nagrades_nagradesmodules%5Baction%5D=transcript&tx_nagrades_nagradesmodules%5Bcontroller%5D=Notenverwaltung&tx_nagrades_nagradesmodules%5BcurriculumId%5D=161&tx_nagrades_nagradesmodules%5Blang%5D=de&cHash=1f08230e8aedd6f54255c728bbd29c19"
	dbFile        = "grades.db"
)

type Grade struct {
	Module          string
	Grade           string
	OccurrenceIndex int
}

func main() {
	// Load .env
	godotenv.Load()

	// Init DB
	db, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Create table if not exists - V2 Schema
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS grades_v2 (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		module_name TEXT NOT NULL,
		grade TEXT,
		occurrence_index INTEGER,
		status TEXT,
		updated_at TEXT,
		UNIQUE(module_name, occurrence_index)
	);`)
	if err != nil {
		log.Fatal(err)
	}

	// Setup Client with CookieJar ONCE to persist session
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		log.Fatal(err)
	}
	client := &http.Client{
		Jar:     jar,
		Timeout: 60 * time.Second,
	}

	for {
		// Reload env to get fresh interval/credentials
		godotenv.Load()
		username := os.Getenv("CIS_USERNAME")
		password := os.Getenv("CIS_PASSWORD")
		intervalStr := os.Getenv("CHECK_INTERVAL")
		customTranscriptURL := os.Getenv("TRANSCRIPT_URL")

		targetURL := transcriptURL
		if customTranscriptURL != "" {
			targetURL = customTranscriptURL
		}

		interval := 60
		if intervalStr != "" {
			if val, err := strconv.Atoi(intervalStr); err == nil {
				interval = val
			}
		}

		if username == "" || password == "" {
			log.Println("CIS_USERNAME and CIS_PASSWORD must be set in .env. Waiting...")
			time.Sleep(1 * time.Minute)
			continue
		}

		log.Println("Starting check cycle...")
		checkGrades(db, client, username, password, targetURL)
		log.Printf("Check finished. Sleeping for %d minutes.\n", interval)

		time.Sleep(time.Duration(interval) * time.Minute)
	}
}

func checkGrades(db *sql.DB, client *http.Client, username, password, targetURL string) {
	// 1. Try to access transcript directly
	log.Println("Checking session validity...")
	resp, err := client.Get(targetURL)
	if err != nil {
		log.Println("Failed to access transcript URL:", err)
		return
	}
	defer resp.Body.Close()

	// Check if we got the PDF or a login page
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/pdf") {
		log.Println("Session expired or invalid (got HTML instead of PDF). Logging in...")

		// Perform Login
		if err := performLogin(client, username, password); err != nil {
			log.Println("Login failed:", err)
			return
		}

		// Retry fetching transcript
		log.Println("Retrying transcript download...")
		resp, err = client.Get(targetURL)
		if err != nil {
			log.Println("Failed to download transcript after login:", err)
			return
		}
		defer resp.Body.Close()
	} else {
		log.Println("Session is valid.")
	}

	if resp.StatusCode != 200 {
		log.Printf("Failed to download transcript, status: %d\n", resp.StatusCode)
		return
	}

	// Save PDF locally with progress
	log.Println("Downloading PDF...")
	pdfData, err := downloadWithProgress(resp)
	if err != nil {
		log.Println("Download failed:", err)
		return
	}

	err = os.WriteFile("grades.pdf", pdfData, 0644)
	if err != nil {
		log.Println(err)
		return
	}
	log.Println("PDF downloaded successfully.")

	// Parse PDF
	log.Println("Parsing PDF content...")
	content, err := readPdf("grades.pdf")
	if err != nil {
		log.Println("Failed to read PDF:", err)
		return
	}

	// Extract Grades and Compare
	newGrades := extractGrades(content)
	log.Printf("Found %d grades in PDF. Checking against database...\n", len(newGrades))

	// Check if DB is empty (First Run)
	var count int
	err = db.QueryRow("SELECT count(*) FROM grades_v2").Scan(&count)
	if err != nil {
		log.Println("DB Error checking count:", err)
		return
	}

	isFirstRun := count == 0
	if isFirstRun {
		log.Println("Database is empty. Performing initial silent sync...")
	}

	for _, g := range newGrades {
		var exists int
		err = db.QueryRow("SELECT count(*) FROM grades_v2 WHERE module_name = ? AND occurrence_index = ?", g.Module, g.OccurrenceIndex).Scan(&exists)
		if err != nil {
			log.Println("DB Error:", err)
			continue
		}

		if exists == 0 {
			// New grade entry
			if !isFirstRun {
				fmt.Printf("New Grade found: %s - %s\n", g.Module, g.Grade)
				log.Printf("New Grade found: %s - %s\n", g.Module, g.Grade)
				notify(g.Module, g.Grade)
			} else {
				log.Printf("Silently adding initial grade: %s - %s\n", g.Module, g.Grade)
			}

			_, err = db.Exec("INSERT INTO grades_v2 (module_name, grade, occurrence_index, status, updated_at) VALUES (?, ?, ?, ?, ?)",
				g.Module, g.Grade, g.OccurrenceIndex, "new", time.Now().Format(time.RFC3339))
			if err != nil {
				log.Println("Insert Error:", err)
			}

		} else {
			// Check if grade changed
			var currentGrade string
			err = db.QueryRow("SELECT grade FROM grades_v2 WHERE module_name = ? AND occurrence_index = ?", g.Module, g.OccurrenceIndex).Scan(&currentGrade)

			if err == nil && currentGrade != g.Grade {
				fmt.Printf("Grade updated: %s - %s -> %s\n", g.Module, currentGrade, g.Grade)
				log.Printf("Grade updated: %s - %s -> %s\n", g.Module, currentGrade, g.Grade)

				_, err = db.Exec("UPDATE grades_v2 SET grade = ?, updated_at = ? WHERE module_name = ? AND occurrence_index = ?",
					g.Grade, time.Now().Format(time.RFC3339), g.Module, g.OccurrenceIndex)
				if err != nil {
					log.Println("Update Error:", err)
				}

				notify(g.Module, g.Grade)
			}
		}
	}

	if isFirstRun {
		log.Println("Initial silent sync complete. Notifications will be enabled for future runs.")
	}
}

func performLogin(client *http.Client, username, password string) error {
	log.Println("Fetching login page...")
	resp, err := client.Get(loginURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return err
	}

	form := doc.Find("form").FilterFunction(func(i int, s *goquery.Selection) bool {
		action, exists := s.Attr("action")
		return exists && strings.Contains(action, "login")
	})

	if form.Length() == 0 {
		form = doc.Find("form").FilterFunction(func(i int, s *goquery.Selection) bool {
			return s.Find("input[name='user']").Length() > 0
		})
	}

	if form.Length() == 0 {
		return fmt.Errorf("could not find login form")
	}

	action, _ := form.Attr("action")
	if !strings.HasPrefix(action, "http") {
		u, _ := url.Parse(loginURL)
		rel, _ := url.Parse(action)
		action = u.ResolveReference(rel).String()
	}

	data := url.Values{}
	data.Set("user", username)
	data.Set("pass", password)

	form.Find("input[type=hidden]").Each(func(i int, s *goquery.Selection) {
		name, _ := s.Attr("name")
		val, _ := s.Attr("value")
		data.Set(name, val)
	})

	log.Println("Submitting login credentials...")
	resp, err = client.PostForm(action, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Verify login success
	// Usually, a successful login redirects or returns a page without the login form.
	// We can check if the response URL is different or if we see a "Logout" link.
	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyString := string(bodyBytes)

	if strings.Contains(bodyString, "Anmeldefehler") || strings.Contains(bodyString, "Login fehlgeschlagen") {
		return fmt.Errorf("login failed: invalid credentials")
	}

	log.Println("Login successful.")
	return nil
}

func downloadWithProgress(resp *http.Response) ([]byte, error) {
	size, _ := strconv.Atoi(resp.Header.Get("Content-Length"))

	var buf bytes.Buffer
	buffer := make([]byte, 32*1024) // 32KB buffer
	var downloaded int

	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			buf.Write(buffer[:n])
			downloaded += n
			if size > 0 {
				percent := float64(downloaded) / float64(size) * 100
				// Log every 20%
				if downloaded%(size/5) < n {
					log.Printf("Downloading: %.0f%% (%d/%d bytes)\n", percent, downloaded, size)
				}
			} else {
				// If no content length, just log bytes
				if downloaded%(1024*1024) < n { // Every 1MB
					log.Printf("Downloaded: %d bytes...\n", downloaded)
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func readPdf(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var buf bytes.Buffer
	b, err := r.GetPlainText()
	if err != nil {
		return "", err
	}
	buf.ReadFrom(b)
	return buf.String(), nil
}

func extractGrades(text string) []Grade {
	var grades []Grade
	lines := strings.Split(text, "\n")

	// Clean lines
	var cleanLines []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			cleanLines = append(cleanLines, l)
		}
	}

	// Regex for Module ID (e.g., I169)
	reID := regexp.MustCompile(`^I\d+$`)

	var currentModuleID string
	var currentModuleName string
	var currentGradeParts []string

	// Track occurrences of each module to handle retakes
	moduleOccurrences := make(map[string]int)

	for i := 0; i < len(cleanLines); i++ {
		line := cleanLines[i]

		if reID.MatchString(line) {
			// Save previous module if exists
			if currentModuleID != "" {
				gradeStr := strings.Join(currentGradeParts, " ")
				if gradeStr == "" {
					gradeStr = "?" // Should not happen usually
				}

				// Calculate occurrence index
				idx := moduleOccurrences[currentModuleName]
				moduleOccurrences[currentModuleName]++

				grades = append(grades, Grade{
					Module:          currentModuleName,
					Grade:           gradeStr,
					OccurrenceIndex: idx,
				})
			}

			// Start new module
			currentModuleID = line
			if i+1 < len(cleanLines) {
				currentModuleName = cleanLines[i+1]
				i++ // Skip name line
			} else {
				currentModuleName = "Unknown"
			}
			currentGradeParts = []string{}
		} else {
			// Collecting grade info
			// Skip CP lines
			if strings.HasSuffix(line, " CP") || line == "Credits" {
				continue
			}
			// Skip other potential headers if they appear (heuristic)
			if line == "Note" || line == "Name" || line == "ModulNr" {
				continue
			}

			// Stop if we hit the footer
			if strings.HasPrefix(line, "Diese Notenübersicht ist kein Zeugnis") ||
				strings.HasPrefix(line, "Der derzeitige Notendurchschnitt") {
				break
			}

			// Append to grade
			if currentModuleID != "" {
				currentGradeParts = append(currentGradeParts, line)
			}
		}
	}

	// Add last module
	if currentModuleID != "" {
		gradeStr := strings.Join(currentGradeParts, " ")

		idx := moduleOccurrences[currentModuleName]
		moduleOccurrences[currentModuleName]++

		grades = append(grades, Grade{
			Module:          currentModuleName,
			Grade:           gradeStr,
			OccurrenceIndex: idx,
		})
	}

	return grades
}

func notify(module, grade string) {
	msg := fmt.Sprintf("New Grade: %s - %s", module, grade)

	// Local Notification
	cmd := exec.Command("notify-send", "GradeChecker", msg)
	cmd.Run()

	// Discord Notification
	discordEnabled := os.Getenv("DISCORD_ENABLED")
	if discordEnabled == "true" || discordEnabled == "1" || discordEnabled == "yes" {
		mode := os.Getenv("DISCORD_MODE")
		if mode == "dm" {
			token := os.Getenv("DISCORD_BOT_TOKEN")
			userID := os.Getenv("DISCORD_USER_ID")
			if token != "" && userID != "" {
				sendDiscordDM(token, userID, msg)
			}
		} else {
			webhookURL := os.Getenv("DISCORD_WEBHOOK_URL")
			if webhookURL != "" {
				sendDiscordNotification(webhookURL, msg)
			}
		}
	}
}

func sendDiscordNotification(webhookURL, message string) {
	payload := fmt.Sprintf(`{"content": "%s"}`, message)
	resp, err := http.Post(webhookURL, "application/json", strings.NewReader(payload))
	if err != nil {
		log.Println("Failed to send Discord notification:", err)
		return
	}
	defer resp.Body.Close()
}

func sendDiscordDM(token, userID, message string) {
	// 1. Create DM Channel
	createDMPayload := fmt.Sprintf(`{"recipient_id": "%s"}`, userID)
	req, err := http.NewRequest("POST", "https://discord.com/api/v10/users/@me/channels", strings.NewReader(createDMPayload))
	if err != nil {
		log.Println("Failed to create DM request:", err)
		return
	}
	req.Header.Set("Authorization", "Bot "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Failed to create DM channel:", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Failed to create DM channel (Status %d): %s\n", resp.StatusCode, string(body))
		return
	}

	// Parse Channel ID
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Println("Failed to parse DM channel response:", err)
		return
	}

	channelID, ok := result["id"].(string)
	if !ok {
		log.Println("Failed to get channel ID from response")
		return
	}

	// 2. Send Message
	msgPayload := fmt.Sprintf(`{"content": "%s"}`, message)
	req, err = http.NewRequest("POST", fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages", channelID), strings.NewReader(msgPayload))
	if err != nil {
		log.Println("Failed to create message request:", err)
		return
	}
	req.Header.Set("Authorization", "Bot "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err = client.Do(req)
	if err != nil {
		log.Println("Failed to send DM:", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Failed to send DM (Status %d): %s\n", resp.StatusCode, string(body))
	}
}
