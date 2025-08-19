package v1handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"dnk.com/hoc-golang/telegram"
	"github.com/gin-gonic/gin"
)

type TelegramHandler struct {
	Client *telegram.TelegramClient
}

func NewTelegramHandler(token string) *TelegramHandler {
	return &TelegramHandler{
		Client: telegram.NewTelegramClient(token),
	}
}

// We'll simply log and (optionally) send a text reply if message present.
func (h *TelegramHandler) HandleUpdate(c *gin.Context) {
	var update telegram.Update
	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update payload", "detail": err.Error()})
		return
	}

	if update.Message != nil {
		chatID := update.Message.Chat.ID
		text := update.Message.Text
		reply := fmt.Sprintf("Bạn gửi: %s", text)
		data, status, err := h.Client.SendMessageRaw(chatID, reply)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":           "failed to send reply",
				"detail":          err.Error(),
				"telegram_status": status,
				"telegram_body":   string(data)})
			return
		}
		c.Data(http.StatusOK, "application/json", data)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "no action taken"})
}

// ---- Request structs ----
type SendMessageRequest struct {
	ChatID interface{} `json:"chat_id" form:"chat_id"`
	Text   string      `json:"text" form:"text" binding:"required"`
}

type SendMediaRequest struct {
	ChatID   interface{} `json:"chat_id" form:"chat_id"`
	FilePath string      `json:"file_path" form:"file_path"`
	Caption  string      `json:"caption" form:"caption"`
}

// normalizeChatID accepts string/number/nil and returns int64
func normalizeChatID(chatID interface{}) (int64, error) {
	switch v := chatID.(type) {
	case string:
		return strconv.ParseInt(v, 10, 64)
	case float64:
		return int64(v), nil
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	case nil:
		envChatID := os.Getenv("TELEGRAM_CHAT_ID")
		if envChatID == "" {
			return 0, fmt.Errorf("missing chat_id and TELEGRAM_CHAT_ID not set")
		}
		return strconv.ParseInt(envChatID, 10, 64)
	default:
		return 0, fmt.Errorf("chat_id must be string or number")
	}
}

// parseMediaRequest supports application/json, x-www-form-urlencoded, multipart/form-data
func parseMediaRequest(c *gin.Context) (int64, string, string, error) {
	ct := c.ContentType()

	// JSON or x-www-form-urlencoded share the same struct
	if strings.HasPrefix(ct, "application/json") || strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
		var req SendMediaRequest
		// try bind both json and form
		if strings.HasPrefix(ct, "application/json") {
			if err := c.ShouldBindJSON(&req); err != nil {
				return 0, "", "", err
			}
		} else {
			if err := c.ShouldBind(&req); err != nil {
				return 0, "", "", err
			}
		}
		chatID, err := normalizeChatID(req.ChatID)
		if err != nil {
			return 0, "", "", err
		}
		// filePath may be a URL (remote) or local path on server
		return chatID, req.FilePath, req.Caption, nil
	}

	// multipart/form-data: handle file upload
	if strings.HasPrefix(ct, "multipart/form-data") {
		chatIDStr := c.PostForm("chat_id")
		if chatIDStr == "" {
			env := os.Getenv("TELEGRAM_CHAT_ID")
			if env == "" {
				return 0, "", "", fmt.Errorf("missing chat_id in form and TELEGRAM_CHAT_ID not set")
			}
			chatIDStr = env
		}
		chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
		if err != nil {
			return 0, "", "", err
		}
		caption := c.PostForm("caption")

		// file can be provided as field "file" — save to temp and return path
		file, errFile := c.FormFile("file")
		if errFile == nil && file != nil {
			dst := filepath.Join(os.TempDir(), file.Filename)
			if saveErr := c.SaveUploadedFile(file, dst); saveErr != nil {
				return 0, "", "", saveErr
			}
			return chatID, dst, caption, nil
		}

		// fallback: client may pass file_path form field (URL or server path)
		fp := c.PostForm("file_path")
		if fp == "" {
			return 0, "", "", fmt.Errorf("no file uploaded and file_path not provided")
		}
		return chatID, fp, caption, nil
	}

	return 0, "", "", fmt.Errorf("unsupported Content-Type: %s", ct)
}

// ---- SendMessage handler (supports json, form-urlencoded, multipart) ----
func (h *TelegramHandler) SendMessage(c *gin.Context) {
	ct := c.ContentType()

	var req SendMessageRequest
	switch {
	case strings.HasPrefix(ct, "application/json"):
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	case strings.HasPrefix(ct, "application/x-www-form-urlencoded"):
		if err := c.ShouldBind(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	case strings.HasPrefix(ct, "multipart/form-data"):
		req.ChatID = c.PostForm("chat_id")
		req.Text = c.PostForm("text")
		if req.Text == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "text required"})
			return
		}
		// ---- Bổ sung dùng postFormURLEncoded ----
		chatID, err := normalizeChatID(req.ChatID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		form := map[string]string{
			"chat_id": strconv.FormatInt(chatID, 10),
			"text":    req.Text,
		}
		data, status, err := h.Client.PostFormURLEncoded("sendMessage", form)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":  err.Error(),
				"status": status,
				"body":   string(data),
			})
			return
		}
		c.Data(http.StatusOK, "application/json", data)

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported content-type", "type": ct})
		return
	}

	chatID, err := normalizeChatID(req.ChatID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Gọi Telegram API và nhận raw data
	data, status, err := h.Client.SendMessageRaw(chatID, req.Text)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  "telegram error",
			"detail": err.Error(),
			"status": status,
			"body":   string(data),
		})
		return
	}

	// ----- Log databack ra terminal -----
	var pretty interface{}
	if json.Unmarshal(data, &pretty) == nil {
		b, _ := json.MarshalIndent(pretty, "", "  ")
		fmt.Printf("[Telegram SendMessage Response]\n%s\n", string(b))
	} else {
		fmt.Printf("[Telegram SendMessage Response] %s\n", string(data))
	}

	// Trả response về client
	c.Data(http.StatusOK, "application/json", data)
}

// ---- Generic media handler factory ----
func (h *TelegramHandler) sendMediaWrapper(c *gin.Context, sendFunc func(int64, string, string) ([]byte, int, error)) {
	chatID, filePath, caption, err := parseMediaRequest(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// if file path is local but doesn't exist, return error (except if it's a URL)
	if !(strings.HasPrefix(filePath, "http://") || strings.HasPrefix(filePath, "https://")) {
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "file not found on server", "file": filePath})
			return
		}
	}
	data, status, err := sendFunc(chatID, filePath, caption)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "telegram error", "detail": err.Error(), "status": status, "body": string(data)})
		return
	}
	c.Data(http.StatusOK, "application/json", data)
}

func (h *TelegramHandler) SendPhoto(c *gin.Context) {
	h.sendMediaWrapper(c, h.Client.SendPhotoRaw)
}
func (h *TelegramHandler) SendAudio(c *gin.Context) {
	h.sendMediaWrapper(c, h.Client.SendAudioRaw)
}
func (h *TelegramHandler) SendDocument(c *gin.Context) {
	h.sendMediaWrapper(c, h.Client.SendDocumentRaw)
}
func (h *TelegramHandler) SendVideo(c *gin.Context) {
	h.sendMediaWrapper(c, h.Client.SendVideoRaw)
}
func (h *TelegramHandler) SendAnimation(c *gin.Context) {
	h.sendMediaWrapper(c, h.Client.SendAnimationRaw)
}
func (h *TelegramHandler) SendVoice(c *gin.Context) {
	h.sendMediaWrapper(c, h.Client.SendVoiceRaw)
}

// ---- GetUpdates (support offset & reset) ----
// ---- GetUpdates (support offset & reset) ----
func (h *TelegramHandler) GetUpdates(c *gin.Context) {
	// Parse query parameters
	reset, _ := strconv.ParseBool(c.DefaultQuery("reset", "false"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100")) // Telegram default is 100

	// Reset offset if requested
	if reset {
		offset = 0
	}

	// Build query params
	params := fmt.Sprintf("?timeout=10&limit=%d", limit)
	if offset > 0 {
		params += fmt.Sprintf("&offset=%d", offset)
	}

	// Fetch updates (NOTE: 3 return values)
	data, status, err := h.Client.FetchUpdatesRaw(params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  "failed to fetch updates",
			"detail": err.Error(),
			"status": status,
			"body":   string(data),
			"params": params,
		})
		return
	}

	// Parse response to get last update_id
	var updates struct {
		OK     bool              `json:"ok"`
		Result []telegram.Update `json:"result"`
	}
	if err := json.Unmarshal(data, &updates); err == nil && len(updates.Result) > 0 {
		lastUpdateID := updates.Result[len(updates.Result)-1].UpdateID
		c.Header("X-Last-Update-ID", strconv.Itoa(lastUpdateID))
	}

	c.Data(http.StatusOK, "application/json", data)
}

// ---- Fetch external API and send its body to a secondary chat ----
func (h *TelegramHandler) FetchAndSendToTelegram(c *gin.Context) {
	apiURL := c.DefaultQuery("url", "")
	if apiURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing url query parameter"})
		return
	}

	// TELEGRAM_CHAT_ID
	mainChatIDStr := os.Getenv("TELEGRAM_CHAT_ID") // Sửa ở đây
	if mainChatIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "TELEGRAM_CHAT_ID not set in env"})
		return
	}
	mainChatID, err := strconv.ParseInt(mainChatIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid TELEGRAM_CHAT_ID"})
		return
	}

	// Phần còn lại giữ nguyên, chỉ thay trackChatID bằng mainChatID
	client := http.Client{Timeout: 12 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("call API error: %v", err)})
		return
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("read body error: %v", err)})
		return
	}

	textToSend := string(bodyBytes)
	var v interface{}
	if json.Unmarshal(bodyBytes, &v) == nil {
		if pretty, err := json.MarshalIndent(v, "", "  "); err == nil {
			textToSend = string(pretty)
		}
	}

	const maxLen = 3800
	if len(textToSend) > maxLen {
		textToSend = textToSend[:maxLen] + "\n... [truncated]"
	}

	// Sửa trackChatID -> mainChatID ở đây
	data, status, err := h.Client.SendMessageRaw(mainChatID, fmt.Sprintf("📡 Data from %s:\n%s", apiURL, textToSend))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  fmt.Sprintf("send telegram error: %v", err),
			"status": status,
			"body":   string(data),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Data fetched and sent to main Telegram group",
		"data":    textToSend,
		"tg_resp": json.RawMessage(data),
	})
}

// ---- Member management handlers ----
type BanRequest struct {
	ChatID    interface{} `json:"chat_id" form:"chat_id" binding:"required"`
	UserID    int64       `json:"user_id" form:"user_id" binding:"required"`
	UntilDate int64       `json:"until_date" form:"until_date"` // 0 = forever
}

func (h *TelegramHandler) BanMember(c *gin.Context) {
	var req BanRequest
	if err := bindAny(c, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	chatID, err := normalizeChatID(req.ChatID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	data, status, err := h.Client.BanChatMemberRaw(chatID, req.UserID, req.UntilDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "status": status, "body": string(data)})
		return
	}
	c.Data(http.StatusOK, "application/json", data)
}

type UnbanRequest struct {
	ChatID       interface{} `json:"chat_id" form:"chat_id" binding:"required"`
	UserID       int64       `json:"user_id" form:"user_id" binding:"required"`
	OnlyIfBanned bool        `json:"only_if_banned" form:"only_if_banned"`
}

func (h *TelegramHandler) UnbanMember(c *gin.Context) {
	var req UnbanRequest
	if err := bindAny(c, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	chatID, err := normalizeChatID(req.ChatID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	data, status, err := h.Client.UnbanChatMemberRaw(chatID, req.UserID, req.OnlyIfBanned)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "status": status, "body": string(data)})
		return
	}
	c.Data(http.StatusOK, "application/json", data)
}

type InviteRequest struct {
	ChatID      interface{} `json:"chat_id" form:"chat_id" binding:"required"`
	Name        string      `json:"name" form:"name"`
	ExpireDate  int64       `json:"expire_date" form:"expire_date"`
	MemberLimit int         `json:"member_limit" form:"member_limit"`
}

func (h *TelegramHandler) CreateInviteLink(c *gin.Context) {
	var req InviteRequest
	if err := bindAny(c, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	chatID, err := normalizeChatID(req.ChatID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	data, status, err := h.Client.CreateChatInviteLinkRaw(chatID, req.Name, req.ExpireDate, req.MemberLimit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "status": status, "body": string(data)})
		return
	}
	c.Data(http.StatusOK, "application/json", data)
}

// bindAny helper: binds JSON or form
func bindAny(c *gin.Context, v interface{}) error {
	ct := c.ContentType()
	if strings.HasPrefix(ct, "application/json") {
		return c.ShouldBindJSON(v)
	}
	return c.ShouldBind(v)
}

type MessageActionRequest struct {
	ChatID    interface{} `json:"chat_id" form:"chat_id" binding:"required"`
	MessageID int         `json:"message_id" form:"message_id" binding:"required"`
	Text      string      `json:"text"` // chỉ dùng cho Edit
}

func (h *TelegramHandler) PinMessage(c *gin.Context) {
	var req MessageActionRequest
	if err := bindAny(c, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	chatID, err := normalizeChatID(req.ChatID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	data, status, err := h.Client.PinChatMessageRaw(chatID, req.MessageID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "status": status, "body": string(data)})
		return
	}

	c.Data(http.StatusOK, "application/json", data)
}

func (h *TelegramHandler) UnpinMessage(c *gin.Context) {
	var req MessageActionRequest
	if err := bindAny(c, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	chatID, err := normalizeChatID(req.ChatID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	data, status, err := h.Client.UnpinChatMessageRaw(chatID, req.MessageID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "status": status, "body": string(data)})
		return
	}

	c.Data(http.StatusOK, "application/json", data)
}

func (h *TelegramHandler) EditMessage(c *gin.Context) {
	var req MessageActionRequest
	if err := bindAny(c, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	chatID, err := normalizeChatID(req.ChatID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Text == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "text required for edit"})
		return
	}

	data, status, err := h.Client.EditMessageTextRaw(chatID, req.MessageID, req.Text)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "status": status, "body": string(data)})
		return
	}

	c.Data(http.StatusOK, "application/json", data)
}

func (h *TelegramHandler) DeleteMessage(c *gin.Context) {
	var req MessageActionRequest
	if err := bindAny(c, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	chatID, err := normalizeChatID(req.ChatID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	data, status, err := h.Client.DeleteMessageRaw(chatID, req.MessageID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "status": status, "body": string(data)})
		return
	}

	c.Data(http.StatusOK, "application/json", data)
}
