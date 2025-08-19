package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"dnk.com/hoc-golang/middleware"
)

// TelegramClient quản lý token và gửi request tới Telegram API
type TelegramClient struct {
	Token  string
	Client *http.Client
}

// NewTelegramClient khởi tạo client với token
func NewTelegramClient(token string) *TelegramClient {
	return &TelegramClient{
		Token:  token,
		Client: &http.Client{},
	}
}

// --------------------- Helpers chung ---------------------
func (c *TelegramClient) buildURL(method string) string {
	return fmt.Sprintf("https://api.telegram.org/bot%s/%s", c.Token, method)
}

// postJSON gửi payload JSON, trả về raw body + status code
func (c *TelegramClient) postJSON(method string, payload interface{}) ([]byte, int, error) {
	if c.Token == "" {
		return nil, 0, fmt.Errorf("⚠️ Telegram token empty")
	}
	url := c.buildURL(method)

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal error: %v", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, 0, fmt.Errorf("%s request error: %v", method, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return respBody, resp.StatusCode, fmt.Errorf("❌ %s failed, status: %s, response: %s", method, resp.Status, string(respBody))
	}
	return respBody, resp.StatusCode, nil
}

// postFormURLEncoded gửi application/x-www-form-urlencoded
func (c *TelegramClient) PostFormURLEncoded(method string, data map[string]string) ([]byte, int, error) {
	if c.Token == "" {
		return nil, 0, fmt.Errorf("⚠️ Telegram token empty")
	}
	url := c.buildURL(method)

	form := make([]string, 0, len(data))
	for k, v := range data {
		form = append(form, fmt.Sprintf("%s=%s", k, v))
	}
	body := strings.Join(form, "&")
	resp, err := http.Post(url, "application/x-www-form-urlencoded", strings.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("%s request error: %v", method, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return respBody, resp.StatusCode, fmt.Errorf("❌ %s failed, status: %s, response: %s", method, resp.Status, string(respBody))
	}
	return respBody, resp.StatusCode, nil
}

// postFile gửi multipart/form-data, skip nếu file không tồn tại
func (c *TelegramClient) postFile(method string, chatID int64, fieldName, filePath string, extra map[string]string) ([]byte, int, error) {
	if c.Token == "" {
		return nil, 0, fmt.Errorf("⚠️ Telegram token empty")
	}

	// Nếu là URL thì gửi qua JSON
	if strings.HasPrefix(filePath, "http://") || strings.HasPrefix(filePath, "https://") {
		payload := map[string]interface{}{"chat_id": chatID}
		switch method {
		case "sendPhoto":
			payload["photo"] = filePath
		case "sendAnimation":
			payload["animation"] = filePath
		case "sendVideo":
			payload["video"] = filePath
		case "sendAudio":
			payload["audio"] = filePath
		case "sendDocument":
			payload["document"] = filePath
		case "sendVoice":
			payload["voice"] = filePath
		default:
			payload["file"] = filePath
		}
		for k, v := range extra {
			payload[k] = v
		}
		return c.postJSON(method, payload)
	}

	// Skip nếu file không tồn tại
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		fmt.Printf("⚠️ Skip file %s (not exist)\n", filePath)
		return nil, 0, nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, 0, fmt.Errorf("open file error: %v", err)
	}
	defer file.Close()

	var b bytes.Buffer
	writer := multipart.NewWriter(&b)
	_ = writer.WriteField("chat_id", strconv.FormatInt(chatID, 10))
	for k, v := range extra {
		_ = writer.WriteField(k, v)
	}

	part, err := writer.CreateFormFile(fieldName, filepath.Base(filePath))
	if err != nil {
		return nil, 0, fmt.Errorf("create form file error: %v", err)
	}
	if _, err = io.Copy(part, file); err != nil {
		return nil, 0, fmt.Errorf("copy file error: %v", err)
	}

	if err = writer.Close(); err != nil {
		return nil, 0, fmt.Errorf("writer close error: %v", err)
	}

	req, err := http.NewRequest("POST", c.buildURL(method), &b)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("%s request error: %v", method, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return respBody, resp.StatusCode, fmt.Errorf("❌ %s failed, status: %s, response: %s", method, resp.Status, string(respBody))
	}
	fmt.Printf("✅ %s sent: %s\n", method, filePath)
	return respBody, resp.StatusCode, nil
}

// --------------------- SendMessage ---------------------
func (c *TelegramClient) SendMessageRaw(chatID int64, text string) ([]byte, int, error) {
	const maxLen = 4000
	if len(text) > maxLen {
		text = text[:maxLen] + "\n... [truncated]"
	}
	payload := map[string]interface{}{"chat_id": chatID, "text": text}
	data, status, err := c.postJSON("sendMessage", payload)
	if err != nil {
		middleware.LogTelegramError("❌ SendMessageRaw failed", err, map[string]interface{}{"chat_id": chatID, "text": text})
		return data, status, err
	}
	fmt.Printf("✅ Message sent to %d\n", chatID)
	return data, status, nil
}

func (c *TelegramClient) SendMessage(chatID int64, text string) error {
	_, _, err := c.SendMessageRaw(chatID, text)
	return err
}

// --------------------- Media Send Wrappers ---------------------
func (c *TelegramClient) SendPhotoRaw(chatID int64, filePath, caption string) ([]byte, int, error) {
	return c.postFile("sendPhoto", chatID, "photo", filePath, map[string]string{"caption": caption})
}
func (c *TelegramClient) SendAudioRaw(chatID int64, filePath, caption string) ([]byte, int, error) {
	return c.postFile("sendAudio", chatID, "audio", filePath, map[string]string{"caption": caption})
}
func (c *TelegramClient) SendDocumentRaw(chatID int64, filePath, caption string) ([]byte, int, error) {
	return c.postFile("sendDocument", chatID, "document", filePath, map[string]string{"caption": caption})
}
func (c *TelegramClient) SendVideoRaw(chatID int64, filePath, caption string) ([]byte, int, error) {
	return c.postFile("sendVideo", chatID, "video", filePath, map[string]string{"caption": caption})
}
func (c *TelegramClient) SendAnimationRaw(chatID int64, filePath, caption string) ([]byte, int, error) {
	return c.postFile("sendAnimation", chatID, "animation", filePath, map[string]string{"caption": caption})
}
func (c *TelegramClient) SendVoiceRaw(chatID int64, filePath, caption string) ([]byte, int, error) {
	return c.postFile("sendVoice", chatID, "voice", filePath, map[string]string{"caption": caption})
}

// --------------------- GetUpdates ---------------------

// GetUpdatesParams chứa các tham số cho GetUpdates
type GetUpdatesParams struct {
	Offset  int // ID update lớn nhất đã xử lý + 1
	Limit   int // Số updates tối đa (1-100)
	Timeout int // Thời gian chờ (giây)
}

// GetUpdatesV2 phiên bản cải tiến với xử lý offset tự động
func (c *TelegramClient) GetUpdatesV2(params GetUpdatesParams) ([]Update, error) {
	// Validate các tham số
	if params.Limit < 0 || params.Limit > 100 {
		return nil, fmt.Errorf("limit must be between 1-100")
	}
	if params.Timeout < 0 || params.Timeout > 90 {
		return nil, fmt.Errorf("timeout must be between 0-90 seconds")
	}

	payload := map[string]interface{}{
		"offset": params.Offset,
	}
	if params.Limit > 0 {
		payload["limit"] = params.Limit
	}
	if params.Timeout > 0 {
		payload["timeout"] = params.Timeout
	}

	respBody, _, err := c.postJSON("getUpdates", payload)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}

	var result struct {
		Ok     bool     `json:"ok"`
		Result []Update `json:"result"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode error: %v", err)
	}
	
	if !result.Ok {
		return nil, fmt.Errorf("telegram API error")
	}
	return result.Result, nil
}

// FetchUpdatesRaw 
func (c *TelegramClient) FetchUpdatesRaw(params string) ([]byte, int, error) {
    if c.Token == "" {
        return nil, 0, fmt.Errorf("⚠️ Telegram token empty")
    }
    url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates%s", c.Token, params)
    resp, err := http.Get(url)
    if err != nil {
        return nil, 0, err
    }
    defer resp.Body.Close()
    resBody, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, resp.StatusCode, err
    }
    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        return resBody, resp.StatusCode, fmt.Errorf("❌ getUpdates error: status %s, body: %s", resp.Status, string(resBody))
    }
    return resBody, resp.StatusCode, nil
}
// GetUpdatesWithOffset (giữ nguyên để tương thích)
func (c *TelegramClient) GetUpdatesWithOffset(offset, limit int) ([]Update, error) {
	payload := map[string]interface{}{
		"offset": offset,
		"limit":  limit,
	}

	respBody, _, err := c.postJSON("getUpdates", payload)
	if err != nil {
		return nil, err
	}

	var result struct {
		Ok     bool     `json:"ok"`
		Result []Update `json:"result"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode error: %v", err)
	}
	if !result.Ok {
		return nil, fmt.Errorf("telegram API returned not ok")
	}
	return result.Result, nil
}

// --------------------- Member management ---------------------
func (c *TelegramClient) BanChatMemberRaw(chatID, userID, untilDate int64) ([]byte, int, error) {
	payload := map[string]interface{}{"chat_id": chatID, "user_id": userID, "until_date": untilDate}
	return c.postJSON("banChatMember", payload)
}
func (c *TelegramClient) UnbanChatMemberRaw(chatID, userID int64, onlyIfBanned bool) ([]byte, int, error) {
	payload := map[string]interface{}{"chat_id": chatID, "user_id": userID, "only_if_banned": onlyIfBanned}
	return c.postJSON("unbanChatMember", payload)
}
func (c *TelegramClient) CreateChatInviteLinkRaw(chatID int64, name string, expireDate int64, memberLimit int) ([]byte, int, error) {
	payload := map[string]interface{}{"chat_id": chatID}
	if name != "" {
		payload["name"] = name
	}
	if expireDate > 0 {
		payload["expire_date"] = expireDate
	}
	if memberLimit > 0 {
		payload["member_limit"] = memberLimit
	}
	return c.postJSON("createChatInviteLink", payload)
}
 // --------------------- Pin / Unpin / Edit / Delete Messages ---------------------

// PinChatMessageRaw pin một message trong chat
func (c *TelegramClient) PinChatMessageRaw(chatID int64, messageID int) ([]byte, int, error) {
	payload := map[string]interface{}{
		"chat_id":              chatID,
		"message_id":           messageID,
		"disable_notification": false, // default false
	}
	return c.postJSON("pinChatMessage", payload)
}

// UnpinChatMessageRaw unpin một message trong chat
func (c *TelegramClient) UnpinChatMessageRaw(chatID int64, messageID int) ([]byte, int, error) {
	payload := map[string]interface{}{
		"chat_id":    chatID,
		"message_id": messageID,
	}
	return c.postJSON("unpinChatMessage", payload)
}

// EditMessageTextRaw chỉnh sửa text của message đã gửi
func (c *TelegramClient) EditMessageTextRaw(chatID int64, messageID int, newText string) ([]byte, int, error) {
	payload := map[string]interface{}{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       newText,
	}
	return c.postJSON("editMessageText", payload)
}

// DeleteMessageRaw xóa message đã gửi
func (c *TelegramClient) DeleteMessageRaw(chatID int64, messageID int) ([]byte, int, error) {
	payload := map[string]interface{}{
		"chat_id":    chatID,
		"message_id": messageID,
	}
	return c.postJSON("deleteMessage", payload)
}
