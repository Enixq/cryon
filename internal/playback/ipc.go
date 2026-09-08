package playback

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// sendCommandLocked отправляет команду mpv по IPC и ждёт ответа.
// Вызывается под c.mu.
func (c *Controller) sendCommandLocked(command string, args ...any) error {
	if strings.TrimSpace(c.ipcPath) == "" {
		return errUnavailable
	}
	conn, err := dialIPC(c.ipcPath, 2*time.Second)
	if err != nil {
		return fmt.Errorf("подключение к mpv IPC не удалось: %w", err)
	}
	defer conn.Close()

	payload := map[string]any{"command": append([]any{command}, args...)}
	if err := json.NewEncoder(conn).Encode(payload); err != nil {
		return fmt.Errorf("отправка команды mpv не удалась: %w", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var response map[string]any
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&response); err != nil {
		return fmt.Errorf("чтение ответа mpv не удалось: %w", err)
	}
	if value, ok := response["error"].(string); ok && value != "success" {
		return fmt.Errorf("команда mpv завершилась ошибкой: %s", value)
	}
	return nil
}

// queryFloatLocked запрашивает у mpv числовое свойство (get_property).
// Вызывается под c.mu.
func (c *Controller) queryFloatLocked(property string) (float64, error) {
	if strings.TrimSpace(c.ipcPath) == "" {
		return 0, errUnavailable
	}
	conn, err := dialIPC(c.ipcPath, 2*time.Second)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	payload := map[string]any{"command": []any{"get_property", property}}
	if err := json.NewEncoder(conn).Encode(payload); err != nil {
		return 0, err
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var response map[string]any
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&response); err != nil {
		return 0, err
	}
	if value, ok := response["error"].(string); ok && value != "success" {
		return 0, fmt.Errorf("mpv get_property %s завершился ошибкой: %s", property, value)
	}
	switch v := response["data"].(type) {
	case float64:
		return v, nil
	case int64:
		return float64(v), nil
	default:
		return 0, errors.New("нет числового значения в ответе mpv")
	}
}

// queryPositionLocked запрашивает текущую позицию воспроизведения (сек).
func (c *Controller) queryPositionLocked() (float64, error) {
	return c.queryFloatLocked("playback-time")
}

// queryDurationLocked запрашивает полную длительность текущего трека (сек).
// У сетевых потоков (YouTube/Yandex) она известна только после старта
// воспроизведения — до этого duration=0 и фронтенд не может строить полоску.
func (c *Controller) queryDurationLocked() (float64, error) {
	return c.queryFloatLocked("duration")
}

// queryEOFLocked проверяет свойство eof-reached mpv (конец трека достигнут).
// Вызывается под c.mu.
func (c *Controller) queryEOFLocked() (bool, error) {
	if strings.TrimSpace(c.ipcPath) == "" {
		return false, errUnavailable
	}
	conn, err := dialIPC(c.ipcPath, 2*time.Second)
	if err != nil {
		return false, err
	}
	defer conn.Close()

	payload := map[string]any{"command": []any{"get_property", "eof-reached"}}
	if err := json.NewEncoder(conn).Encode(payload); err != nil {
		return false, err
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var response map[string]any
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&response); err != nil {
		return false, err
	}
	if v, ok := response["data"].(bool); ok {
		return v, nil
	}
	return false, nil
}

// waitForIPC ждёт готовности IPC-сокета mpv в течение timeout.
func waitForIPC(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		conn, err := dialIPC(path, 500*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("mpv IPC %s не готов: %w", path, lastErr)
	}
	return fmt.Errorf("mpv IPC %s не готов", path)
}

// dialIPC подключается к IPC mpv: named pipe в Windows, unix-сокет иначе.
func dialIPC(path string, timeout time.Duration) (net.Conn, error) {
	if strings.HasPrefix(path, `\\.\pipe\`) {
		return dialNamedPipe(path, timeout)
	}
	return net.DialTimeout("unix", path, timeout)
}
