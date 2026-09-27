// Умный дом Яндекса: список розеток и вкл/выкл. Порт app/services/yandex.py.
// В тестах сеть подменяется — сюда в реальный API не ходим.
package yandex

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const api = "https://api.iot.yandex.net/v1.0"
const onOff = "devices.capabilities.on_off"

var client = &http.Client{Timeout: 10 * time.Second}

type YandexError struct{ msg string }

func (e *YandexError) Error() string { return e.msg }

// YandexAuthError — токен не принят (401/403): отозван, истёк или без нужных прав.
type YandexAuthError struct{ msg string }

func (e *YandexAuthError) Error() string { return e.msg }

func NewYandexError(msg string) *YandexError     { return &YandexError{msg} }
func NewAuthError(msg string) *YandexAuthError   { return &YandexAuthError{msg} }

type Device struct {
	ID   string
	Name string
	Room *string
	Type string
}

var ErrYandexUnavailable = errors.New("Умный дом Яндекса недоступен")

func call(token, method, path string, body any) (map[string]any, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, api+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, ErrYandexUnavailable
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, ErrYandexUnavailable
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, &YandexAuthError{"Яндекс не принял токен"}
	}
	if resp.StatusCode != 200 {
		return nil, &YandexError{fmt.Sprintf("Яндекс ответил ошибкой %d", resp.StatusCode)}
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, ErrYandexUnavailable
	}
	if status, _ := payload["status"].(string); status != "ok" {
		msg, _ := payload["message"].(string)
		if msg == "" {
			msg = "Яндекс вернул ошибку"
		}
		return nil, &YandexError{msg}
	}
	return payload, nil
}

func ListDevices(token string) ([]Device, error) {
	data, err := call(token, "GET", "/user/info", nil)
	if err != nil {
		return nil, err
	}
	roomNames := map[string]string{}
	if rooms, ok := data["rooms"].([]any); ok {
		for _, r := range rooms {
			rm, _ := r.(map[string]any)
			id, _ := rm["id"].(string)
			name, _ := rm["name"].(string)
			roomNames[id] = name
		}
	}
	var out []Device
	if devs, ok := data["devices"].([]any); ok {
		for _, d := range devs {
			dev, _ := d.(map[string]any)
			hasOnOff := false
			if caps, ok := dev["capabilities"].([]any); ok {
				for _, c := range caps {
					capr, _ := c.(map[string]any)
					if t, _ := capr["type"].(string); t == onOff {
						hasOnOff = true
					}
				}
			}
			if !hasOnOff {
				continue
			}
			var room *string
			if rid, _ := dev["room"].(string); rid != "" {
				nm := roomNames[rid]
				room = &nm
			}
			dtyp, _ := dev["type"].(string)
			name, _ := dev["name"].(string)
			id, _ := dev["id"].(string)
			out = append(out, Device{ID: id, Name: name, Room: room, Type: dtyp})
		}
	}
	return out, nil
}

func SetOn(token, deviceID string, on bool) error {
	body := map[string]any{
		"devices": []any{
			map[string]any{
				"id": deviceID,
				"actions": []any{
					map[string]any{"type": onOff, "state": map[string]any{"instance": "on", "value": on}},
				},
			},
		},
	}
	data, err := call(token, "POST", "/devices/actions", body)
	if err != nil {
		return err
	}
	if devs, ok := data["devices"].([]any); ok {
		for _, d := range devs {
			dev, _ := d.(map[string]any)
			if caps, ok := dev["capabilities"].([]any); ok {
				for _, c := range caps {
					capr, _ := c.(map[string]any)
					if st, ok := capr["state"].(map[string]any); ok {
						if ar, ok := st["action_result"].(map[string]any); ok {
							if s, _ := ar["status"].(string); s == "ERROR" {
								msg, _ := ar["error_message"].(string)
								if msg == "" {
									if code, _ := ar["error_code"].(string); code != "" {
										msg = code
									} else {
										msg = "Розетка не выполнила команду"
									}
								}
								return &YandexError{msg}
							}
						}
					}
				}
			}
		}
	}
	return nil
}
