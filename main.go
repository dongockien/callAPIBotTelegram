package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"text/tabwriter"

	v1handler "dnk.com/hoc-golang/internal/api/v1/handler"
	v2handler "dnk.com/hoc-golang/internal/api/v2/handler"
	"dnk.com/hoc-golang/middleware"
	"dnk.com/hoc-golang/telegram"
	"dnk.com/hoc-golang/utils"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	// ---------- Load .env ----------
	if err := godotenv.Load(); err != nil {
		log.Fatal("❌ Không thể load file .env")
	}

	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatIDStr := os.Getenv("TELEGRAM_CHAT_ID")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Convert chatID sang int64
	var chatID int64
	if chatIDStr != "" {
		var err error
		chatID, err = strconv.ParseInt(chatIDStr, 10, 64)
		if err != nil {
			log.Fatalf("❌ Invalid TELEGRAM_CHAT_ID: %v", err)
		}
	}

	// ---------- Tạo thư mục logs ----------
	if err := os.MkdirAll("logs", 0755); err != nil {
		log.Fatalf("❌ Cannot create logs dir: %v", err)
	}

	// Log file chính
	logFile, err := os.OpenFile("logs/app.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("❌ Cannot open log file: %v", err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	// ---------- Validator ----------
	if err := utils.RegisterValidators(); err != nil {
		panic(err)
	}

	// ---------- Scheduler setup ----------
	var scheduler *v1handler.Scheduler
	if botToken != "" && chatID != 0 {
		tgClient := telegram.NewTelegramClient(botToken)

		// Interval minutes có thể test nhanh 0.1 phút (~6s)
		intervalMinutes := 5.0
		if v := os.Getenv("SCHEDULER_INTERVAL"); v != "" {
			if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
				intervalMinutes = n
			}
		}

		runCount := 0
		if v := os.Getenv("SCHEDULER_RUNS"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				runCount = n
			}
		}

		retryCount := 2
		retryDelaySec := 5
		logFilePath := "logs/api_check.log"

		// Tạo Scheduler
		s, err := v1handler.NewScheduler(
			strconv.FormatInt(chatID, 10),
			tgClient,
			intervalMinutes, // float64
			runCount,
			retryCount,
			retryDelaySec,
			logFilePath,
		)
		if err != nil {
			log.Fatalf("❌ Failed to create scheduler: %v", err)
		}
		scheduler = s

		log.Println("✅ Scheduler initialized (not started yet)")

		// In cấu hình scheduler đẹp
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "Interval (minutes)\tRuns\tRetry Count\tRetry Delay (s)\n")
		fmt.Fprintf(w, "%.2f\t%d\t%d\t%d\n", intervalMinutes, runCount, retryCount, retryDelaySec)
		w.Flush()

		// Run scheduler ngay khi startup nếu bật SCHEDULER_RUN_IMMEDIATE=1
		if v := os.Getenv("SCHEDULER_RUN_IMMEDIATE"); v == "1" {
			if err := scheduler.Start(); err != nil {
				log.Printf("❌ Scheduler failed to start immediately: %v", err)
			} else {
				log.Println("✅ Scheduler started immediately at startup")
			}
		}
	} else {
		log.Println("⚠ Scheduler not initialized — missing TELEGRAM_BOT_TOKEN or TELEGRAM_CHAT_ID")
	}

	// ---------- Gin Router ----------
	r := gin.Default()
	telegramHandler := v1handler.NewTelegramHandler(botToken)

	r.Use(
		middleware.LoggerMiddleware(),
		middleware.ApiKeyMiddleware(),
		middleware.RateLimitingMiddleware(),
	)

	v1 := r.Group("/api/v1")
	{
		telegramGroup := v1.Group("/telegram")

		// Text/Media endpoints
		telegramGroup.POST("/send", telegramHandler.SendMessage)
		telegramGroup.GET("/updates", telegramHandler.GetUpdates)
		telegramGroup.GET("/fetch-send", telegramHandler.FetchAndSendToTelegram)
		telegramGroup.POST("/webhook", telegramHandler.HandleUpdate)
		telegramGroup.POST("/sendPhoto", telegramHandler.SendPhoto)
		telegramGroup.POST("/sendAudio", telegramHandler.SendAudio)
		telegramGroup.POST("/sendDocument", telegramHandler.SendDocument)
		telegramGroup.POST("/sendVideo", telegramHandler.SendVideo)
		telegramGroup.POST("/sendAnimation", telegramHandler.SendAnimation)
		telegramGroup.POST("/sendVoice", telegramHandler.SendVoice)
		telegramGroup.POST("/pinMessage", telegramHandler.PinMessage)
		telegramGroup.POST("/unpinMessage", telegramHandler.UnpinMessage)
		telegramGroup.POST("/editMessage", telegramHandler.EditMessage)
		telegramGroup.POST("/deleteMessage", telegramHandler.DeleteMessage)

		// Member management
		telegramGroup.POST("/banMember", telegramHandler.BanMember)
		telegramGroup.POST("/unbanMember", telegramHandler.UnbanMember)
		telegramGroup.POST("/createInviteLink", telegramHandler.CreateInviteLink)

		// Test message
		telegramGroup.GET("/test", func(c *gin.Context) {
			if botToken == "" || chatID == 0 {
				c.JSON(500, gin.H{"error": "TELEGRAM_BOT_TOKEN or TELEGRAM_CHAT_ID not configured"})
				return
			}
			_, _, err := telegram.NewTelegramClient(botToken).SendMessageRaw(chatID, "🚀 Test message from server")
			if err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
			c.JSON(200, gin.H{"message": "Test message sent"})
		})

		// Scheduler control endpoints
		telegramGroup.POST("/scheduler/start", func(c *gin.Context) {
			if scheduler == nil {
				c.JSON(500, gin.H{"error": "Scheduler not initialized"})
				return
			}
			if scheduler.IsRunning() {
				c.JSON(400, gin.H{"error": "Scheduler already running"})
				return
			}
			if err := scheduler.Start(); err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
			c.JSON(200, gin.H{"message": "Scheduler started"})
			log.Println("✅ Scheduler started by API call")
		})
		telegramGroup.POST("/scheduler/stop", func(c *gin.Context) {
			if scheduler == nil || !scheduler.IsRunning() {
				c.JSON(400, gin.H{"error": "Scheduler not running"})
				return
			}
			scheduler.Stop()
			c.JSON(200, gin.H{"message": "Scheduler stopped"})
			log.Println("✅ Scheduler stopped by API call")
		})
	}

	// ---------- API v2 ----------
	v2 := r.Group("/api/v2")
	{
		user := v2.Group("/users")
		{
			UserHandlerV2 := v2handler.NewUserHandler()
			user.GET("", UserHandlerV2.GetUsersV2)
			user.GET("/:id", UserHandlerV2.GetUsersByIdV2)
			user.POST("", UserHandlerV2.PostUsersV2)
			user.PUT("/:id", UserHandlerV2.PutUsersV2)
			user.DELETE("/:id", UserHandlerV2.DeleteUsersV2)
		}
	}

	// Static files
	r.StaticFS("/images", gin.Dir("./uploads", false))

	// ---------- Start server ----------
	fmt.Printf("🚀 Server running on port %s\n", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("❌ Failed to start server: %v", err)
	}
}
