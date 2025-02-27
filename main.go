package main

import (
	"bot/config"
	"bufio"
	"encoding/csv"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/telebot.v3"
)

var (
	requestLimit    = 3
	banLimit        = 5
	rateLimitWindow = 5 * time.Second
	banDuration     = 30 * time.Second

	userRequests = make(map[int64][]time.Time)
	bannedUsers  = make(map[int64]time.Time)
	mutex        = sync.Mutex{}
	
	// Create a logger
	logger *log.Logger
)

func initLogger() {
	// Create logs directory if it doesn't exist
	os.MkdirAll("logs", os.ModePerm)
	
	// Open log file with append mode
	logFile, err := os.OpenFile("logs/bot.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	
	// Create logger that writes to both stdout and file
	logger = log.New(logFile, "", log.LstdFlags)
}

func logInfo(format string, v ...interface{}) {
	logMessage := fmt.Sprintf(format, v...)
	logger.Println("INFO:", logMessage)
	fmt.Println("INFO:", logMessage)
}

func logError(format string, v ...interface{}) {
	logMessage := fmt.Sprintf(format, v...)
	logger.Println("ERROR:", logMessage)
	fmt.Println("ERROR:", logMessage)
}

func isValidURL(input string) bool {
	parsedURL, err := url.ParseRequestURI(input)
	if err != nil {
		return false
	}

	// Faqat http yoki https protokollari bo'lishi kerak
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return false
	}

	return true
}

func isRateLimited(userID int64) bool {
	mutex.Lock()
	defer mutex.Unlock()

	now := time.Now()

	if banEndTime, banned := bannedUsers[userID]; banned {
		if now.Before(banEndTime) {
			return true
		}
		delete(bannedUsers, userID)
	}

	requests := userRequests[userID]
	filteredRequests := []time.Time{}

	for _, reqTime := range requests {
		if now.Sub(reqTime) < rateLimitWindow {
			filteredRequests = append(filteredRequests, reqTime)
		}
	}

	if len(filteredRequests) >= banLimit {
		bannedUsers[userID] = now.Add(banDuration)
		delete(userRequests, userID)
		return true
	}

	if len(filteredRequests) >= requestLimit {
		return true
	}

	filteredRequests = append(filteredRequests, now)
	userRequests[userID] = filteredRequests
	return false
}

func logRequest(user *telebot.User, url string) {
	logFile := "downloads/requests.csv"
	os.MkdirAll("downloads", os.ModePerm)

	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logError("Could not open log file: %v", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	loc, _ := time.LoadLocation("Asia/Tashkent")
	currentTime := time.Now().In(loc).Format("2006-01-02 15:04:05")

	record := []string{
		fmt.Sprintf("%d", user.ID),
		user.Username,
		user.FirstName,
		user.LastName,
		url,
		currentTime,
	}

	if err := writer.Write(record); err != nil {
		logError("Could not write to log file: %v", err)
	}
	
	logInfo("User %d (@%s) requested: %s", user.ID, user.Username, url)
}

func getServiceType(urlStr string) string {
	if strings.Contains(urlStr, "youtube.com") || strings.Contains(urlStr, "youtu.be") {
		return "YouTube"
	} else if strings.Contains(urlStr, "instagram.com") {
		return "Instagram"
	} else if strings.Contains(urlStr, "tiktok.com") {
		return "TikTok"
	} else if strings.Contains(urlStr, "facebook.com") || strings.Contains(urlStr, "fb.com") {
		return "Facebook"
	}
	return "Unknown"
}

func downloadMedia(userID int64, username string, url string, progress chan int) (string, error) {
	service := getServiceType(url)
	logInfo("Starting download for User %d (@%s): %s [%s]", userID, username, url, service)
	
	// Create a unique download directory for each request
	downloadID := fmt.Sprintf("%d_%d", userID, time.Now().Unix())
	downloadDir := fmt.Sprintf("downloads/%s", downloadID)
	os.MkdirAll(downloadDir, os.ModePerm)
	
	outputTemplate := downloadDir + "/%(title)s.%(ext)s"
	
	// Updated yt-dlp command with more options
	cmd := exec.Command(
		"yt-dlp",
		"--verbose",            // More verbose output
		"--force-ipv4",         // Force IPv4 (can help with some network issues)
		"--socket-timeout", "30",  // Longer socket timeout
		"--retries", "10",      // More retries
		"--fragment-retries", "10", // More fragment retries
		"--no-check-certificate", // Skip certificate validation
		"-f", "mp4/bestvideo[ext=mp4]+bestaudio[ext=m4a]/mp4", // Prefer mp4 format
		"--merge-output-format", "mp4", // Force final output to mp4
		"-o", outputTemplate,
		"--newline",
		url,
	)

	// Log the exact command
	logInfo("Running command: %s", strings.Join(cmd.Args, " "))
	
	stderr, _ := cmd.StderrPipe()
	stdout, _ := cmd.StdoutPipe()
	
	err := cmd.Start()
	if err != nil {
		logError("Failed to start yt-dlp command: %v", err)
		return "", err
	}

	// Log stderr for debugging
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			logInfo("yt-dlp stderr: %s", line)
		}
	}()
	
	// Process stdout for progress and logging
	go func() {
		scanner := bufio.NewScanner(stdout)
		re := regexp.MustCompile(`(\d+\.\d+)%`)
		lastProgress := 0

		for scanner.Scan() {
			line := scanner.Text()
			logInfo("yt-dlp stdout: %s", line)
			
			match := re.FindStringSubmatch(line)
			if len(match) > 1 {
				if percent, err := strconv.ParseFloat(match[1], 64); err == nil {
					currentProgress := int(percent)
					if currentProgress != lastProgress {
						progress <- currentProgress
						lastProgress = currentProgress
					}
				}
			}
		}
	}()

	err = cmd.Wait()
	if err != nil {
		logError("yt-dlp command failed: %v", err)
		return "", err
	}

	files, err := filepath.Glob(downloadDir + "/*.mp4")
	if err != nil || len(files) == 0 {
		logError("No downloaded files found in directory: %s", downloadDir)
		
		// Check if there are any other files
		allFiles, _ := filepath.Glob(downloadDir + "/*")
		if len(allFiles) > 0 {
			logInfo("Found non-mp4 files: %v", allFiles)
			
			// Try to convert if we found some other format
			for _, file := range allFiles {
				if filepath.Ext(file) != ".mp4" && filepath.Ext(file) != "" {
					outputFile := file[:len(file)-len(filepath.Ext(file))] + ".mp4"
					logInfo("Attempting to convert %s to %s", file, outputFile)
					
					convertCmd := exec.Command("ffmpeg", "-i", file, "-c:v", "libx264", "-preset", "fast", "-c:a", "aac", "-b:a", "192k", outputFile)
					convertOutput, convertErr := convertCmd.CombinedOutput()
					logInfo("ffmpeg output: %s", string(convertOutput))
					
					if convertErr == nil {
						logInfo("Conversion successful, using: %s", outputFile)
						return outputFile, nil
					} else {
						logError("Conversion failed: %v", convertErr)
					}
				}
			}
		}
		
		return "", fmt.Errorf("Yuklab olingan fayl topilmadi")
	}

	logInfo("Download completed successfully: %s", files[0])
	return files[0], nil
}

func main() {
	// Initialize logger
	initLogger()
	logInfo("Starting Media Download Bot")
	
	cnf, err := config.NewConfig()
	if err != nil {
		logError("Failed to load config: %v", err)
		return
	}
	botToken := cnf.TelegramToken
	logInfo("Config loaded successfully")

	pref := telebot.Settings{
		Token:  botToken,
		Poller: &telebot.LongPoller{Timeout: 12 * time.Second},
	}

	bot, err := telebot.NewBot(pref)
	if err != nil {
		logError("Failed to create bot: %v", err)
		return
	}
	logInfo("Bot created successfully")

	// Create downloads directory
	os.MkdirAll("downloads", os.ModePerm)

	// Start komandasi uchun handler
	bot.Handle("/start", func(c telebot.Context) error {
		user := c.Sender()
		logInfo("User %d (@%s) sent /start command", user.ID, user.Username)
		
		welcomeMsg := `🎉 Assalomu alaykum! Media Download botiga xush kelibsiz! 

📱 Men sizga quyidagi xizmatlarni taqdim etaman:
- YouTube video/audio
- Instagram post/reels
- TikTok video
- Facebook video

🔍 Ishlash tartibi:
1. Yuklab olmoqchi bo'lgan link/URL ni yuboring
2. Men sizga faylni yuklab beraman!

⚡️ Tezkor va ishonchli xizmat kafolati bilan!

🤖 Bot @media_download_any_bot`

		// Sticker yuborish
		sticker := &telebot.Sticker{File: telebot.File{FileID: "CAACAgIAAxkBAAEBuhplOYW_AAFAaNNv-7rjG-QnNJlorgkAAmUBAAIw1J0RZQ1MeHG3J0I0BA"}}
		c.Send(sticker)

		return c.Send(welcomeMsg)
	})

	bot.Handle("/version", func(c telebot.Context) error {
		user := c.Sender()
		logInfo("User %d (@%s) checked version", user.ID, user.Username)
		
		// Check yt-dlp version
		versionCmd := exec.Command("yt-dlp", "--version")
		versionOutput, versionErr := versionCmd.CombinedOutput()
		
		versionText := "yt-dlp version: "
		if versionErr != nil {
			versionText += "Error checking version"
			logError("Failed to check yt-dlp version: %v", versionErr)
		} else {
			versionText += strings.TrimSpace(string(versionOutput))
			logInfo("yt-dlp version: %s", strings.TrimSpace(string(versionOutput)))
		}
		
		return c.Send(versionText)
	})

	bot.Handle(telebot.OnText, func(c telebot.Context) error {
		user := c.Sender()
		url := c.Text()
		
		logInfo("Received URL from User %d (@%s): %s", user.ID, user.Username, url)
		
		if isRateLimited(user.ID) {
			logInfo("User %d (@%s) is rate limited", user.ID, user.Username)
			return c.Send("⚠️ Siz vaqtinchalik bloklandingiz yoki juda ko'p so'rov yubordingiz. Iltimos, keyinroq urinib ko'ring.")
		}

		// URL haqiqatdan ham to'g'rimi?
		if !isValidURL(url) {
			logInfo("User %d (@%s) sent invalid URL: %s", user.ID, user.Username, url)
			return c.Send("❌ Iltimos, to'g'ri URL manzil yuboring! Masalan: https://example.com/video")
		}

		logRequest(user, url)

		statusMsg, err := c.Bot().Send(c.Chat(), "🔄 URL tekshirilmoqda...")
		if err != nil {
			logError("Failed to send initial status message: %v", err)
			return err
		}

		service := getServiceType(url)
		c.Bot().Edit(statusMsg, fmt.Sprintf("🔍 %s dan media yuklab olinmoqda...", service))

		progress := make(chan int)
		done := make(chan bool)

		go func() {
			lastProgress := -1
			for p := range progress {
				if p != lastProgress {
					progressMsg := fmt.Sprintf("⏳ %s dan yuklanmoqda... %d%%", service, p)
					c.Bot().Edit(statusMsg, progressMsg)
					logInfo("Download progress for User %d: %d%%", user.ID, p)
					lastProgress = p
				}
			}
			done <- true
		}()

		filePath, err := downloadMedia(user.ID, user.Username, url, progress)
		close(progress)
		<-done

		if err != nil {
			logError("Download failed for User %d (@%s): %v", user.ID, user.Username, err)
			return c.Send(fmt.Sprintf("❌ Xatolik: faylni yuklab bo'lmadi. Xato: %v", err))
		}

		c.Bot().Edit(statusMsg, "✅ Fayl muvaffaqiyatli yuklandi! Yuborilmoqda...")
		logInfo("Successfully downloaded file for User %d: %s", user.ID, filePath)

		// Get file info
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			logError("Failed to get file info: %v", err)
		} else {
			fileSizeMB := float64(fileInfo.Size()) / 1024.0 / 1024.0
			logInfo("File size: %.2f MB", fileSizeMB)
		}

		video := &telebot.Video{
			File:    telebot.FromDisk(filePath),
			Caption: "✨ @media_download_any_bot orqali yuklab olindi",
		}

		err = c.Send(video)
		if err != nil {
			logError("Failed to send video to User %d: %v", user.ID, err)
			
			// Try sending as a document if video fails
			logInfo("Trying to send as document instead")
			doc := &telebot.Document{
				File:    telebot.FromDisk(filePath),
				Caption: "✨ @media_download_any_bot orqali yuklab olindi",
			}
			
			docErr := c.Send(doc)
			if docErr != nil {
				logError("Failed to send document: %v", docErr)
				return c.Send("❌ Xatolik: faylni yuborib bo'lmadi. Hajmi juda katta bo'lishi mumkin.")
			}
		} else {
			logInfo("Successfully sent video to User %d (@%s)", user.ID, user.Username)
		}

		// Clean up the file
		os.Remove(filePath)
		logInfo("Removed file: %s", filePath)
		return nil
	})

	logInfo("Bot started successfully! 🚀")
	fmt.Println("Bot muvaffaqiyatli ishga tushdi! 🚀")
	bot.Start()
}