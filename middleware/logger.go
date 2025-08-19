package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/natefinch/lumberjack"
	"github.com/rs/zerolog"
	"os"
)

type CustomResponseWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *CustomResponseWriter) Write(data []byte) (n int, err error) {
	w.body.Write(data)
	return w.ResponseWriter.Write(data)
}

func LoggerMiddleware() gin.HandlerFunc {
	logPath := "logs/http.log"

	logger := zerolog.New(&lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    1, // megabytes
		MaxBackups: 5,
		MaxAge:     5,   //days
		Compress:   true, 
		LocalTime: true,
	}).With().Timestamp().Logger()

	return func(ctx *gin.Context) {
		start := time.Now()
		contentType := ctx.GetHeader("Content-Type")
		requestBody := make(map[string]any)
		var formFiles []map[string]any
		//multipart/form-data
		if strings.HasPrefix(contentType, "multipart/form-data") {
			if err := ctx.Request.ParseMultipartForm(32 << 20); err == nil && ctx.Request.MultipartForm != nil {
				//for value
				for key, vals := range ctx.Request.MultipartForm.Value {
					if len(vals) == 1 {
						requestBody[key] = vals[0]
					} else {
						requestBody[key] = vals
					}
				}
				//for file
				for field, files := range ctx.Request.MultipartForm.File {
					for _, f := range files {
						formFiles = append(formFiles, map[string]any{
							"field":        field,
							"filename":     f.Filename,
							"size":         formatFileSize(f.Size),
							"content_type": f.Header.Get("Content-Type"),
						})
					}
				}
				if len(formFiles) > 0 {
					requestBody["form_files"] = formFiles
				}
			}

		} else {
			bodyBytes, err := io.ReadAll(ctx.Request.Body)

			if err != nil {
				logger.Error().Err(err).Msg("Failed to read request body")
			}

			ctx.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

			// application/json
			if strings.HasPrefix(contentType, "application/json") {
				_ = json.Unmarshal(bodyBytes, &requestBody)
			} else {
				//application/x-www-form-urlencoded
				values, _ := url.ParseQuery(string(bodyBytes))
				for key, vals := range values {
					if len(vals) == 1 {
						requestBody[key] = vals[0]
					} else {
						requestBody[key] = vals
					}
				}
			}
		}

		customWriter := &CustomResponseWriter{
			ResponseWriter: ctx.Writer,
			body:           bytes.NewBufferString(""),
		}

		ctx.Writer = customWriter
		ctx.Next()

		duration := time.Since(start)

		statusCode := ctx.Writer.Status()

		responseContentType := ctx.Writer.Header().Get("Content-Type")
		responseBodyRaw := customWriter.body.String()
		var responseBodyParsed interface{}
		if strings.HasPrefix(responseContentType, "image/") {
			responseBodyParsed = "[BINARY DATA]"
		} else if strings.HasPrefix(responseContentType, "application/json") ||
			strings.HasPrefix(strings.TrimSpace(responseBodyRaw), "{") ||
			strings.HasPrefix(strings.TrimSpace(responseBodyRaw), "[") {
			if err := json.Unmarshal([]byte(responseBodyRaw), &responseBodyParsed); err != nil {
				responseBodyParsed = responseBodyRaw
			}
		} else {
			responseBodyParsed = responseBodyRaw
		}
		logEvent := logger.Info()
		if statusCode >= 500 {
			logEvent = logger.Error()
		} else if statusCode >= 400 {
			logEvent = logger.Warn()
		}
		logEvent.
			Str("method", ctx.Request.Method).
			Str("path", ctx.Request.URL.Path).
			Str("query", ctx.Request.URL.RawQuery).
			Str("client_ip", ctx.ClientIP()).
			Str("user_agent", ctx.Request.UserAgent()).
			Str("referer", ctx.Request.Referer()).
			Str("protocol", ctx.Request.Proto).
			Str("host", ctx.Request.Host).
			Str("remote_addr", ctx.Request.RemoteAddr).
			Str("request_uri", ctx.Request.RequestURI).
			Int64("content_length", ctx.Request.ContentLength).
			Interface("headers", ctx.Request.Header).
			Interface("request_body", requestBody).
			//Interface("uploaded_files", formFiles).
			Interface("response", responseBodyParsed).
			Int("status_code", statusCode).
			Int64("duration_ms", duration.Milliseconds()).
			Msg("HTTP Request Log")
	}
}

func formatFileSize(size int64) string {
	switch {
	case size >= 1<<20:
		return fmt.Sprintf("%.2f MB", float64(size)/(1<<20))
	case size >= 1<<10:
		return fmt.Sprintf("%.2f KB", float64(size)/(1<<10))
	default:
		return fmt.Sprintf("%d B", size)
	}
	
}

func LoggerMiddlewareWithColor() gin.HandlerFunc {
    return LoggerMiddleware()
}

// --------------------- Logger riêng cho Telegram ---------------------

var TelegramLogger = zerolog.New(&lumberjack.Logger{
	Filename:   "logs/telegram.log",
	MaxSize:    5, // megabytes
	MaxBackups: 5,
	MaxAge:     7,    //days
	Compress:   true,
	LocalTime:  true,
}).With().Timestamp().Logger()

// Hàm helper để log info
func LogTelegramInfo(msg string, fields map[string]interface{}) {
	event := TelegramLogger.Info()
	for k, v := range fields {
		event.Interface(k, v)
	}
	event.Msg(msg)
}

// Hàm helper để log lỗi
func LogTelegramError(msg string, err error, fields map[string]interface{}) {
	event := TelegramLogger.Error().Err(err)
	for k, v := range fields {
		event.Interface(k, v)
	}
	event.Msg(msg)
}

func init() {
	// Đồng thời ghi log đơn giản ra stdout nữa nếu muốn
	TelegramLogger = TelegramLogger.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})
}
