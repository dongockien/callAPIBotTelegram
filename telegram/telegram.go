package telegram

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// ===== Struct Models =====

type User struct {
	ID           int    `json:"id"`
	IsBot        bool   `json:"is_bot"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name,omitempty"`
	Username     string `json:"username,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type,omitempty"`
}

type Message struct {
	MessageID int    `json:"message_id"`
	From      *User  `json:"from,omitempty"`
	Chat      Chat   `json:"chat"`
	Date      int    `json:"date"`
	Text      string `json:"text,omitempty"`
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
type CallbackQuery struct {
	ID              string   `json:"id"`
	From            User     `json:"from"`
	Message         *Message `json:"message,omitempty"`
	InlineMessageID string   `json:"inline_message_id,omitempty"`
	Data            string   `json:"data"`
}

type Update struct {
	UpdateID      int            `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

type GetUpdatesResponse struct {
	OK     bool     `json:"ok"`
	Result []Update `json:"result"`
}

type SendMessageResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
	Result      struct {
		MessageID int    `json:"message_id"`
		Text      string `json:"text"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	} `json:"result"`
}

// NEW: CreateInviteLinkResponse để parse kết quả tạo link
type CreateInviteLinkResponse struct {
	OK     bool `json:"ok"`
	Result struct {
		InviteLink string `json:"invite_link"`
		Name       string `json:"name"`
		Creator    User   `json:"creator"`
		ExpireDate int64  `json:"expire_date"`
	} `json:"result"`
}

// ===== Utils =====

func ChatIDFromString(s string) int64 {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		fmt.Println("ChatIDFromString error:", err)
		return 0
	}
	return id
}

func (g *GetUpdatesResponse) FromBytes(b []byte) error {
	return json.Unmarshal(b, g)
}

func (s *SendMessageResponse) FromBytes(b []byte) error {
	return json.Unmarshal(b, s)
}

func (c *CreateInviteLinkResponse) FromBytes(b []byte) error {
	return json.Unmarshal(b, c)
}

func ToJSON(v interface{}) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}
