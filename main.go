package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"

	"bot/config"
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
)


func isValidURL(input string) bool {
	parsedURL, err := url.ParseRequestURI(input)
	if err != nil {
		return false
	}

	// Faqat http yoki https protokollari bo‘lishi kerak
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
		fmt.Println("Xatolik: log faylni ochib bo'lmadi:", err)
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
		fmt.Println("Xatolik: log yozib bo'lmadi:", err)
	}
}

func downloadMedia(url string, progress chan int) (string, error) {
	cmd := exec.Command("yt-dlp", "-o", "downloads/%(title)s.%(ext)s", "--newline", url)
	stderr, _ := cmd.StderrPipe()
	err := cmd.Start()
	if err != nil {
		return "", err
	}

	go func() {
		scanner := bufio.NewScanner(stderr)
		re := regexp.MustCompile(`(\d+\.\d+)%`)
		lastProgress := 0

		for scanner.Scan() {
			line := scanner.Text()
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
		return "", err
	}

	files, err := filepath.Glob("downloads/*.mp4")
	if err != nil || len(files) == 0 {
		return "", fmt.Errorf("Yuklab olingan fayl topilmadi")
	}

	return files[0], nil
}

func main() {
	cnf,err := config.NewConfig()
	if err != nil{
		fmt.Println("Env not find:", err)
		return
	}
	botToken := cnf.TelegramToken

	pref := telebot.Settings{
		Token:  botToken,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
	}

	bot, err := telebot.NewBot(pref)
	if err != nil {
		fmt.Println("Botni yaratishda xatolik:", err)
		return
	}

	// Start komandasi uchun handler
	bot.Handle("/start", func(c telebot.Context) error {
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

	bot.Handle(telebot.OnText, func(c telebot.Context) error {
		user := c.Sender()
		if isRateLimited(user.ID) {
			return c.Send("⚠️ Siz vaqtinchalik bloklandingiz yoki juda ko‘p so‘rov yubordingiz. Iltimos, keyinroq urinib ko‘ring.")
		}
	
		url := c.Text()
	
		// URL haqiqatdan ham to‘g‘rimi?
		if !isValidURL(url) {
			return c.Send("❌ Iltimos, to‘g‘ri URL manzil yuboring! Masalan: https://example.com/video")
		}
	
		logRequest(user, url)
	
		statusMsg, err := c.Bot().Send(c.Chat(), "🔄 URL tekshirilmoqda...")
		if err != nil {
			return err
		}
	
		progress := make(chan int)
		done := make(chan bool)
	
		go func() {
			lastProgress := -1
			for p := range progress {
				if p != lastProgress {
					c.Bot().Edit(statusMsg, fmt.Sprintf("⏳ Fayl yuklanmoqda... %d%%", p))
					lastProgress = p
				}
			}
			done <- true
		}()
	
		filePath, err := downloadMedia(url, progress)
		if err != nil {
			close(progress)
			<-done
			return c.Send("❌ Xatolik: faylni yuklab bo‘lmadi.")
		}
	
		close(progress)
		<-done
	
		c.Bot().Edit(statusMsg, "✅ Fayl muvaffaqiyatli yuklandi! Yuborilmoqda...")
	
		video := &telebot.Video{
			File:    telebot.FromDisk(filePath),
			Caption: "✨ @media_download_any_bot orqali yuklab olindi",
		}
	
		err = c.Send(video)
		if err != nil {
			return c.Send("❌ Xatolik: faylni yuborib bo‘lmadi.")
		}
	
		// Faylni tozalash
		os.Remove(filePath)
		return nil
	})
	

	fmt.Println("Bot muvaffaqiyatli ishga tushdi! 🚀")
	bot.Start()
}
