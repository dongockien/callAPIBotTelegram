package v1handler

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"sync"
	"time"

	"dnk.com/hoc-golang/telegram"
	"github.com/sirupsen/logrus"
)

// ---------------- Types ----------------
type apiCallFunc func() (string, int, error)

type APIError struct {
	Op      string
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s failed (status %d): %s", e.Op, e.Status, e.Message)
}

type Scheduler struct {
	chatID          int64
	client          *telegram.TelegramClient
	intervalMinutes float64
	runCount        int
	retryCount      int
	retryDelaySec   int

	mu       sync.Mutex
	running  bool
	stopChan chan struct{}
	wg       sync.WaitGroup
	logger   *logrus.Logger

	apiCalls map[string]apiCallFunc
	apiOrder []string
}

// ANSI màu terminal
const (
	colorReset = "\033[0m"
	colorRed   = "\033[31m"
	colorGreen = "\033[32m"
	colorCyan  = "\033[36m"
)

// Bảng màu cho từng API
var apiColors = map[string]string{
	"SendMessage": "\033[34;1m",
	"SendGIF":     "\033[35;1m",
	"SendVoice":   "\033[36;1m",
	"SendVideo":   "\033[33;1m",
	"GetUpdates":  "\033[37;1m",
}

// ---------------- Constructor ----------------
func NewScheduler(chatIDStr string, client *telegram.TelegramClient,
	intervalMinutes float64, runCount, retryCount, retryDelaySec int, logFilePath string) (*Scheduler, error) {

	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid chatID: %v", err)
	}

	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logFile, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("cannot open log file: %v", err)
	}
	logger.SetOutput(logFile)

	s := &Scheduler{
		chatID:          chatID,
		client:          client,
		intervalMinutes: intervalMinutes,
		runCount:        runCount,
		retryCount:      retryCount,
		retryDelaySec:   retryDelaySec,
		stopChan:        make(chan struct{}),
		logger:          logger,
		apiCalls:        make(map[string]apiCallFunc),
	}

	// Đăng ký API calls
	s.RegisterAPI("SendMessage", s.callSendMessage)
	s.RegisterAPI("SendGIF", s.callSendGIF)
	s.RegisterAPI("SendVoice", s.callSendVoice)
	s.RegisterAPI("SendVideo", s.callSendVideo)
	s.RegisterAPI("GetUpdates", s.callGetUpdates)

	s.logger.WithFields(logrus.Fields{
		"chatID": chatID,
	}).Info("Scheduler initialized")
	return s, nil
}

// ---------------- API registry ----------------
func (s *Scheduler) RegisterAPI(name string, fn apiCallFunc) {
	s.apiCalls[name] = fn
	s.apiOrder = append(s.apiOrder, name)
}

// ---------------- Helper ----------------
func checkFileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return err == nil
}

func (s *Scheduler) logTerminal(level, msg string) {
	ts := time.Now().Format(time.RFC3339)
	var color string
	switch level {
	case "ERR":
		color = colorRed
	case "INF":
		color = colorCyan
	default:
		color = colorGreen
	}
	fmt.Printf("%s %s %s%s%s\n", ts, level, msg, color, colorReset)
	s.logger.WithFields(logrus.Fields{
		"chatID": s.chatID,
		"level":  level,
	}).Info(msg)
}

// ---------------- Retry ----------------
func (s *Scheduler) callWithRetry(apiName string, fn apiCallFunc) (string, int, error) {
	var lastErr error
	var status int
	for attempt := 0; attempt <= s.retryCount; attempt++ {
		select {
		case <-s.stopChan:
			return "", 0, fmt.Errorf("stopped")
		default:
		}

		res, st, err := fn()
		status = st
		if err == nil {
			return res, status, nil
		}

		lastErr = err
		s.logTerminal("ERR", fmt.Sprintf("⚠️ %s attempt %d/%d failed: %v", apiName, attempt+1, s.retryCount+1, err))

		// Exponential backoff
		delay := time.Duration(float64(s.retryDelaySec)*math.Pow(2, float64(attempt))) * time.Second
		select {
		case <-time.After(delay):
		case <-s.stopChan:
			return "", status, fmt.Errorf("stopped during retry")
		}
	}
	return "", status, lastErr
}

// ---------------- Start / Stop ----------------
func (s *Scheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return fmt.Errorf("scheduler already running")
	}
	s.running = true
	s.stopChan = make(chan struct{})
	s.wg.Add(1)
	s.logTerminal("INF", "🚀 Scheduler started")
	go s.runLoop()
	return nil
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopChan)
	s.mu.Unlock()

	s.logTerminal("INF", "🛑 Scheduler stopping...")
	s.wg.Wait()
	s.logTerminal("INF", "✅ Scheduler stopped")
}

func (s *Scheduler) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// ---------------- runLoop ----------------
func (s *Scheduler) runLoop() {
	defer s.wg.Done()
	count := 0

	s.runOnce()
	count++

	interval := time.Duration(s.intervalMinutes * float64(time.Minute))
	timer := time.NewTimer(interval)
	defer timer.Stop()

	for {
		select {
		case <-s.stopChan:
			s.logTerminal("INF", "⏹ Received stop signal, exiting runLoop")
			return
		case <-timer.C:
			s.runOnce()
			count++
			if s.runCount > 0 && count >= s.runCount {
				s.logTerminal("INF", fmt.Sprintf("🏁 Reached runCount limit %d, stopping loop", s.runCount))
				s.Stop()
				return
			}
			timer.Reset(interval)
		}
	}
}

// ---------------- runOnce ----------------
func (s *Scheduler) runOnce() {
	now := time.Now().Format("15:04:05")
	fmt.Printf("%s╔════════════════════════════════════════════════════════════════╗%s\n", colorCyan, colorReset)
	fmt.Printf("%s║ 🚀 Scheduler run started at %s ║%s\n", colorCyan, now, colorReset)
	fmt.Printf("%s╚════════════════════════════════════════════════════════════════╝%s\n", colorCyan, colorReset)

	fmt.Printf("%-4s | %-12s | %-40s\n", "No.", "Type", "Message / Status")
	fmt.Printf("%s---------------------------------------------------------------%s\n", colorCyan, colorReset)

	for i, apiName := range s.apiOrder {
		typeColor := apiColors[apiName]
		res, status, err := s.callWithRetry(apiName, s.apiCalls[apiName])

		var msg, statusColor string
		if err != nil {
			msg = fmt.Sprintf("%v (status=%d)", err, status)
			statusColor = colorRed
		} else {
			msg = fmt.Sprintf("%s (status=%d)", res, status)
			statusColor = colorGreen
		}

		fmt.Printf("%-4d | %s%-12s%s | %s%-40s%s\n", i, typeColor, apiName, colorReset, statusColor, msg, colorReset)
		s.logger.WithFields(logrus.Fields{
			"chatID":    s.chatID,
			"apiName":   apiName,
			"runCount":  i,
			"timestamp": time.Now().Format(time.RFC3339),
		}).Info(msg)

		select {
		case <-s.stopChan:
			return
		default:
			_, _, _ = s.client.SendMessageRaw(s.chatID, fmt.Sprintf("%s: %s", apiName, msg))
		}
	}

	fmt.Printf("%s╔════════════════════════════════════════════════════════════════╗%s\n", colorCyan, colorReset)
	fmt.Printf("%s║ ✔ Scheduler run completed END ║%s\n", colorCyan, colorReset)
	fmt.Printf("%s╚════════════════════════════════════════════════════════════════╝%s\n", colorCyan, colorReset)
}

// ---------------- API calls ----------------
func (s *Scheduler) callSendMessage() (string, int, error) {
	text := "🚀 Scheduler: Test message sent at " + time.Now().Format(time.RFC3339)
	_, status, err := s.client.SendMessageRaw(s.chatID, text)
	if err != nil {
		return "", status, &APIError{"SendMessage", status, err.Error()}
	}
	return "SendMessage ok", status, nil
}

func (s *Scheduler) callSendGIF() (string, int, error) {
	filePath := "./uploads/happy.gif"
	if !checkFileExists(filePath) {
		return "skip sendAnimation - file not found", 0, nil
	}
	_, status, err := s.client.SendAnimationRaw(s.chatID, filePath, "🎉 Scheduler test GIF")
	if err != nil {
		return "", status, &APIError{"SendGIF", status, err.Error()}
	}
	return fmt.Sprintf("sendAnimation sent: %s", filePath), status, nil
}

func (s *Scheduler) callSendVoice() (string, int, error) {
	filePath := "./uploads/test.ogg"
	if !checkFileExists(filePath) {
		return "skip sendVoice - file not found", 0, nil
	}
	_, status, err := s.client.SendVoiceRaw(s.chatID, filePath, "🎙 Scheduler test voice")
	if err != nil {
		return "", status, &APIError{"SendVoice", status, err.Error()}
	}
	return fmt.Sprintf("sendVoice sent: %s", filePath), status, nil
}

func (s *Scheduler) callSendVideo() (string, int, error) {
	filePath := "./uploads/test_small.mp4"
	if !checkFileExists(filePath) {
		return "skip sendVideo - file not found", 0, nil
	}
	_, status, err := s.client.SendVideoRaw(s.chatID, filePath, "🎥 Scheduler test video")
	if err != nil {
		return "", status, &APIError{"SendVideo", status, err.Error()}
	}
	return fmt.Sprintf("sendVideo sent: %s", filePath), status, nil
}

func (s *Scheduler) callGetUpdates() (string, int, error) {
	params := "?offset=0&limit=100&timeout=0"
	body, status, err := s.client.FetchUpdatesRaw(params)
	if err != nil {
		return "", status, &APIError{"GetUpdates", status, err.Error()}
	}

	var result struct {
		Ok     bool              `json:"ok"`
		Result []telegram.Update `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", status, &APIError{"GetUpdates", status, err.Error()}
	}
	if !result.Ok {
		return "", status, &APIError{"GetUpdates", status, "telegram API returned not ok"}
	}

	count := len(result.Result)
	return fmt.Sprintf("GetUpdates ok: %d new", count), status, nil
}
