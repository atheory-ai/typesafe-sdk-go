package typesafe

import (
	"fmt"
	"log"
	"strings"
)

type LogLevel string

const (
	LogDebug LogLevel = "debug"
	LogInfo  LogLevel = "info"
	LogWarn  LogLevel = "warn"
	LogError LogLevel = "error"
	LogOff   LogLevel = "off"
)

var LogLevels = []LogLevel{LogDebug, LogInfo, LogWarn, LogError, LogOff}

type Logger interface {
	Debug(message string, args ...any)
	Info(message string, args ...any)
	Warn(message string, args ...any)
	Error(message string, args ...any)
}

type defaultLogger struct{}

func (defaultLogger) Debug(message string, args ...any) {
	log.Printf("[typesafe-sdk] DEBUG "+message, args...)
}
func (defaultLogger) Info(message string, args ...any) {
	log.Printf("[typesafe-sdk] INFO "+message, args...)
}
func (defaultLogger) Warn(message string, args ...any) {
	log.Printf("[typesafe-sdk] WARN "+message, args...)
}
func (defaultLogger) Error(message string, args ...any) {
	log.Printf("[typesafe-sdk] ERROR "+message, args...)
}

type levelLogger struct {
	sink  Logger
	level LogLevel
}

var logRank = map[LogLevel]int{LogDebug: 0, LogInfo: 1, LogWarn: 2, LogError: 3, LogOff: 4}

func (l levelLogger) enabled(at LogLevel) bool { return logRank[at] >= logRank[l.level] }
func (l levelLogger) Debug(m string, a ...any) {
	if l.enabled(LogDebug) {
		l.sink.Debug(m, a...)
	}
}
func (l levelLogger) Info(m string, a ...any) {
	if l.enabled(LogInfo) {
		l.sink.Info(m, a...)
	}
}
func (l levelLogger) Warn(m string, a ...any) {
	if l.enabled(LogWarn) {
		l.sink.Warn(m, a...)
	}
}
func (l levelLogger) Error(m string, a ...any) {
	if l.enabled(LogError) {
		l.sink.Error(m, a...)
	}
}

func parseLogLevel(value, source string) (LogLevel, error) {
	level := LogLevel(value)
	if _, ok := logRank[level]; ok {
		return level, nil
	}
	values := make([]string, len(LogLevels))
	for i, item := range LogLevels {
		values[i] = string(item)
	}
	return "", &TypeSafeError{Message: fmt.Sprintf(
		"Invalid log level %q from %s. Expected one of: %s.", value, source, strings.Join(values, ", "))}
}

// RedactHeaders returns a copy with credential values masked.
func RedactHeaders(headers map[string]string) map[string]string {
	out := make(map[string]string, len(headers))
	for name, value := range headers {
		switch strings.ToLower(name) {
		case "authorization", "proxy-authorization", "x-api-key":
			parts := strings.Fields(value)
			scheme, secret := "", value
			if len(parts) >= 2 {
				scheme, secret = parts[0]+" ", parts[1]
			}
			tail := ""
			if len(secret) > 8 {
				tail = secret[len(secret)-4:]
			}
			out[name] = scheme + "***" + tail
		case "cookie", "set-cookie":
			out[name] = "***"
		default:
			out[name] = value
		}
	}
	return out
}
