package main

import (
	"bot/config"
	"bufio"
	"encoding/csv"
	"fmt"
	"io/ioutil"
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
	
	// Cookie file path
	instagramCookieFile = "instagram_cookies.txt"
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

// Checks if Instagram cookie file exists
func instagramCookieExists() bool {
	_, err := os.Stat(instagramCookieFile)
	return err == nil
}

// Creates a sample Instagram cookie file with instructions
func createSampleInstagramCookie() error {
	content := `# This is a sample Instagram cookie file
# To use real cookies:
# 1. Log into Instagram in your browser
# 2. Use a browser extension to export cookies (like "Get cookies.txt" for Chrome)
# 3. Replace this file with the exported cookies
# 4. Make sure the file is named "instagram_cookies.txt" in the same directory as the bot

# Format should be: domain_name	TRUE/FALSE	path	secure	expiry	name	value
.instagram.com	TRUE	/	TRUE	1708123456	sessionid	your_session_id_here
.instagram.com	TRUE	/	TRUE	1708123456	ds_user_id	your_user_id_here
`
	return ioutil.WriteFile(instagramCookieFile, []byte(content), 0644)
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
	
	// Basic command arguments
	cmdArgs := []string{
		"--verbose",            // More verbose output
		"--force-ipv4",         // Force IPv4 (can help with some network issues)
		"--socket-timeout", "30",  // Longer socket timeout
		"--retries", "10",      // More retries
		"--fragment-retries", "10", // More fragment retries
		"--no-check-certificate", // Skip certificate validation
	}
	
	// Special handling for Instagram
	if service == "Instagram" {
		if instagramCookieExists() {
			logInfo("Using Instagram cookie file for authentication")
			cmdArgs = append(cmdArgs, "--cookies", instagramCookieFile)
		} else {
			logInfo("Instagram cookie file not found, creating a sample file")
			createSampleInstagramCookie()
			logInfo("Attempting to download without authentication (may fail)")
		}
		
		// For Instagram, use a different format selection
		cmdArgs = append(cmdArgs, "-f", "best")
	} else {
		// For other services, use the optimal format
		cmdArgs = append(cmdArgs, "-f", "mp4/bestvideo[ext=mp4]+bestaudio[ext=m4a]/mp4")
		cmdArgs = append(cmdArgs, "--merge-output-format", "mp4")
	}
	
	// Add output template and URL
	cmdArgs = append(cmdArgs, "-o", outputTemplate, "--newline", url)
	
	// Create the command
	cmd := exec.Command("yt-dlp", cmdArgs...)

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
		
		// If Instagram fails, provide special error message
		if service == "Instagram" {
			return "", fmt.Errorf("Instagram yuklab olishda xatolik. Login ma'lumotlar kerak")
		}
		
		return "", err
	}

	// Search for any video files in the download directory
	videoFiles, _ := filepath.Glob(downloadDir + "/*.mp4")
	if len(videoFiles) > 0 {
		logInfo("Found MP4 file: %s", videoFiles[0])
		return videoFiles[0], nil
	}
	
	// Look for other video formats
	otherVideoFormats := []string{"*.webm", "*.mkv", "*.mov", "*.avi"}
	for _, format := range otherVideoFormats {
		files, _ := filepath.Glob(downloadDir + "/" + format)
		if len(files) > 0 {
			inputFile := files[0]
			outputFile := strings.TrimSuffix(inputFile, filepath.Ext(inputFile)) + ".mp4"
			
			logInfo("Converting %s to MP4 format", inputFile)
			
			convertCmd := exec.Command("ffmpeg", "-i", inputFile, "-c:v", "libx264", "-preset", "fast", "-c:a", "aac", "-b:a", "192k", outputFile)
			convertOutput, convertErr := convertCmd.CombinedOutput()
			
			if convertErr != nil {
				logError("Conversion failed: %v\nOutput: %s", convertErr, string(convertOutput))
				// Try to use original file if conversion fails
				return inputFile, nil
			}
			
			logInfo("Converted to MP4: %s", outputFile)
			return outputFile, nil
		}
	}
	
	// Look for any other files
	allFiles, _ := filepath.Glob(downloadDir + "/*")
	if len(allFiles) > 0 {
		logInfo("Found non-video file: %s", allFiles[0])
		return allFiles[0], nil
	}
	
	return "", fmt.Errorf("Yuklab olingan fayl topilmadi")
}

// Separate function for Instagram direct download
func downloadInstagramWithAPI(url string, downloadDir string) (string, error) {
	logInfo("Attempting Instagram direct download via API for: %s", url)
	
	// Extract Instagram ID from URL
	re := regexp.MustCompile(`/reel/([A-Za-z0-9_-]+)`)
	matches := re.FindStringSubmatch(url)
	if len(matches) < 2 {
		return "", fmt.Errorf("Instagram ID topilmadi")
	}
	
	instagramID := matches[1]
	logInfo("Extracted Instagram ID: %s", instagramID)
	
	// Use a different API service to download Instagram content
	// NOTE: Replace with a real working Instagram API service
	apiURL := fmt.Sprintf("https://instagram-downloader-download-instagram-videos-stories.p.rapidapi.com/index?url=%s", url)
	
	// Create curl command to download using the API
	outputFile := filepath.Join(downloadDir, instagramID+".mp4")
	
	cmd := exec.Command("curl", 
		"-X", "GET", 
		"-H", "X-RapidAPI-Key: YOUR_RAPIDAPI_KEY", // Replace with your RapidAPI key
		"-H", "X-RapidAPI-Host: instagram-downloader-download-instagram-videos-stories.p.rapidapi.com",
		"-o", outputFile,
		apiURL)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		logError("Instagram API download failed: %v\nOutput: %s", err, string(output))
		return "", err
	}
	
	// Check if file exists and has content
	fileInfo, err := os.Stat(outputFile)
	if err != nil || fileInfo.Size() == 0 {
		logError("Instagram API returned empty file or error")
		return "", fmt.Errorf("Instagram API xatolik")
	}
	
	logInfo("Instagram API download successful: %s", outputFile)
	return outputFile, nil
}

func main() {
	// Initialize logger
	initLogger()
	logInfo("Starting Media Download Bot")
	
	// Check for yt-dlp
	ytdlpVersionCmd := exec.Command("yt-dlp", "--version")
	ytdlpVersionOutput, ytdlpVersionErr := ytdlpVersionCmd.CombinedOutput()
	if ytdlpVersionErr != nil {
		logError("yt-dlp not found or not working: %v", ytdlpVersionErr)
		logInfo("Trying to update yt-dlp...")
		
		// Try to install/update yt-dlp
		updateCmd := exec.Command("pip", "install", "--upgrade", "yt-dlp")
		updateOutput, updateErr := updateCmd.CombinedOutput()
		if updateErr != nil {
			logError("Failed to update yt-dlp: %v\nOutput: %s", updateErr, string(updateOutput))
		} else {
			logInfo("yt-dlp updated successfully")
		}
	} else {
		logInfo("yt-dlp version: %s", strings.TrimSpace(string(ytdlpVersionOutput)))
	}
	
	// Check for ffmpeg
	ffmpegVersionCmd := exec.Command("ffmpeg", "-version")
	_, ffmpegVersionErr := ffmpegVersionCmd.CombinedOutput()
	if ffmpegVersionErr != nil {
		logError("ffmpeg not found or not working: %v", ffmpegVersionErr)
		logInfo("Please install ffmpeg for better video handling")
	} else {
		logInfo("ffmpeg found and working")
	}
	
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

	// Admin command to set Instagram cookies
	bot.Handle("/setcookies", func(c telebot.Context) error {
		user := c.Sender()
		logInfo("User %d (@%s) tried to set cookies", user.ID, user.Username)
		
		// Check if user is admin (add your admin user IDs here)
		if user.ID != 12345 && user.ID != 67890 { // Replace with your admin user IDs
			return c.Send("⛔️ Bu buyruq faqat adminlar uchun.")
		}
		
		// Get cookie content from message
		cookieText := strings.TrimPrefix(c.Text(), "/setcookies ")
		if cookieText == "/setcookies" || cookieText == "" {
			return c.Send("❌ Xato format. /setcookies [cookie_matn] ko'rinishida yuboring.")
		}
		
		// Save to cookie file
		err := ioutil.WriteFile(instagramCookieFile, []byte(cookieText), 0644)
		if err != nil {
			logError("Failed to write cookie file: %v", err)
			return c.Send("❌ Cookie faylni saqlashda xatolik yuz berdi.")
		}
		
		logInfo("Instagram cookies updated successfully")
		return c.Send("✅ Instagram cookies muvaffaqiyatli yangilandi!")
	})

	bot.Handle("/version", func(c telebot.Context) error {
		user := c.Sender()
		logInfo("User %d (@%s) checked version", user.ID, user.Username)
		
		// Check yt-dlp version
		versionCmd := exec.Command("yt-dlp", "--version")
		versionOutput, versionErr := versionCmd.CombinedOutput()
		
		var versionText string
		if versionErr != nil {
			versionText = "yt-dlp version: Error checking version"
			logError("Failed to check yt-dlp version: %v", versionErr)
		} else {
			versionText = "yt-dlp version: " + strings.TrimSpace(string(versionOutput))
			logInfo("yt-dlp version: %s", strings.TrimSpace(string(versionOutput)))
		}
		
		// Check ffmpeg version
		ffmpegCmd := exec.Command("ffmpeg", "-version")
		ffmpegOutput, ffmpegErr := ffmpegCmd.CombinedOutput()
		
		if ffmpegErr != nil {
			versionText += "\nffmpeg: Not installed or not working"
		} else {
			// Extract just the first line of ffmpeg version
			ffmpegVersion := strings.Split(string(ffmpegOutput), "\n")[0]
			versionText += "\nffmpeg: " + ffmpegVersion
		}
		
		// Add Instagram cookie status
		if instagramCookieExists() {
			versionText += "\nInstagram cookies: Configured"
		} else {
			versionText += "\nInstagram cookies: Not configured"
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

		// Create a download directory
		downloadID := fmt.Sprintf("%d_%d", user.ID, time.Now().Unix())
		downloadDir := fmt.Sprintf("downloads/%s", downloadID)
		os.MkdirAll(downloadDir, os.ModePerm)

		var filePath string
		
		if service == "Instagram" {
			// First try with yt-dlp 
			filePath, err = downloadMedia(user.ID, user.Username, url, progress)
			if err != nil {
				logInfo("yt-dlp failed for Instagram, trying alternative method")
				
				// If yt-dlp fails for Instagram, try the alternative API method
				close(progress)
				<-done
				
				// Reset progress tracking for alternative method
				progress = make(chan int)
				done = make(chan bool)
				
				go func() {
					// Simulate progress for API method
					for p := 0; p <= 100; p += 10 {
						progress <- p
						time.Sleep(500 * time.Millisecond)
					}
					close(progress)
				}()
				
				// Try alternative API
				filePath, err = downloadInstagramWithAPI(url, downloadDir)
				<-done
			} else {
				close(progress)
				<-done
			}
		} else {
			// For other services, use the standard method
			filePath, err = downloadMedia(user.ID, user.Username, url, progress)
			close(progress)
			<-done
		}

		if err != nil {
			logError("Download failed for User %d (@%s): %v", user.ID, user.Username, err)
			
			if service == "Instagram" {
				return c.Send(`❌ Instagram video yuklab olishda xatolik yuz berdi.

Instagram himoya tizimi tufayli, login ma'lumotlar talab qilinadi.

Administratorga murojaat qiling.`)
			}
			
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

		// Determine file type and send appropriately
		fileExt := strings.ToLower(filepath.Ext(filePath))
		
		if fileExt == ".mp4" || fileExt == ".mov" || fileExt == ".avi" || fileExt == ".mkv" || fileExt == ".webm" {
			// Send as video
			video := &telebot.Video{
				File:    telebot.FromDisk(filePath),
				Caption: "✨ @media_download_any_bot orqali yuklab olindi",
			}
			
			err = c.Send(video)
			if err != nil {
				logError("Failed to send video to User %d: %v", user.ID, err)
				
				// File might be too large, try sending as document
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
			}
		} else if fileExt == ".mp3" || fileExt == ".m4a" || fileExt == ".ogg" || fileExt == ".wav" {
			// Send as audio
			audio := &telebot.Audio{
				File:    telebot.FromDisk(filePath),
				Caption: "✨ @media_download_any_bot orqali yuklab olindi",
			}
			
			err = c.Send(audio)
			if err != nil {
				logError("Failed to send audio: %v", err)
				return c.Send("❌ Xatolik: faylni yuborib bo'lmadi.")
			}
		} else {
			// Send any other file type as document
			doc := &telebot.Document{
				File:    telebot.FromDisk(filePath),
				Caption: "✨ @media_download_any_bot orqali yuklab olindi",
			}
			
			err = c.Send(doc)
			if err != nil {
				logError("Failed to send document: %v", err)
				return c.Send("❌ Xatolik: faylni yuborib bo'lmadi.")
			}
		}
		
		logInfo("Successfully sent media to User %d (@%s)", user.ID, user.Username)

		// Clean up the file
		os.Remove(filePath)
		logInfo("Removed file: %s", filePath)
		return nil
	})

	logInfo("Bot started successfully! 🚀")
	fmt.Println("Bot muvaffaqiyatli ishga tushdi! 🚀")
	bot.Start()
}