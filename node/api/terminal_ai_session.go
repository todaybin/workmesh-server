// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"
)

const terminalAILineClear = byte(21)

func isTerminalEnter(data []byte) bool {
	return bytes.Equal(data, []byte{'\r'}) || bytes.Equal(data, []byte{'\n'}) || bytes.Equal(data, []byte{'\r', '\n'})
}

func handleTerminalAIEnter(ws *streamWebSocket, line string) (bool, []byte, error) {
	settings := loadTerminalAISettings()
	if settings["aiStatus"] != "Enable" {
		return false, nil, nil
	}
	prefix, _ := settings["aiPrefix"].(string)
	current := normalizeTerminalAILine(line, prefix)
	if !matchesTerminalAIPrefix(current, prefix) {
		return false, nil, nil
	}
	prompt := strings.TrimSpace(strings.TrimPrefix(current, prefix))
	if err := writeTerminalAINotice(ws, "info", "AI 正在思考..."); err != nil {
		return true, nil, err
	}
	if prompt == "" {
		_ = writeTerminalAINotice(ws, "error", "请在前缀后输入要转换的命令描述")
		return true, []byte{terminalAILineClear}, nil
	}
	riskCommands := terminalAIRiskList(settings["aiRiskCommands"])
	command, err := terminalAIComplete(prompt)
	if err != nil {
		_ = writeTerminalAINotice(ws, "error", "AI 请求失败："+err.Error())
		return true, []byte{terminalAILineClear}, nil
	}
	command = strings.TrimSpace(command)
	if command == "" {
		_ = writeTerminalAINotice(ws, "error", "AI 没有返回可执行命令")
		return true, []byte{terminalAILineClear}, nil
	}
	if terminalAIRisky(command, riskCommands) {
		_ = writeTerminalAINotice(ws, "error", "已拦截风险命令："+command)
		return true, append([]byte{terminalAILineClear}, []byte("# 已拦截风险命令："+command)...), nil
	}
	if err := validateTerminalAIPaste(command); err != nil {
		_ = writeTerminalAINotice(ws, "error", "AI 返回的命令包含不允许的控制字符")
		return true, []byte{terminalAILineClear}, nil
	}
	if err := writeTerminalAINotice(ws, "success", "思考完成，请回车执行"); err != nil {
		return true, nil, err
	}
	return true, append([]byte{terminalAILineClear}, []byte(command)...), nil
}

func writeTerminalAINotice(ws *streamWebSocket, level, message string) error {
	if ws == nil {
		return nil
	}
	payload, err := json.Marshal(terminalAINotice{Type: "ai_notice", Level: level, Message: message})
	if err != nil {
		return err
	}
	return ws.writeText(payload)
}

func matchesTerminalAIPrefix(line, prefix string) bool {
	line = strings.TrimSpace(line)
	prefix = strings.TrimSpace(prefix)
	return line != "" && prefix != "" && (line == prefix || strings.HasPrefix(line, prefix+" "))
}

func normalizeTerminalAILine(raw, prefix string) string {
	line := strings.TrimSpace(raw)
	prefix = strings.TrimSpace(prefix)
	if line == "" || prefix == "" || matchesTerminalAIPrefix(line, prefix) {
		return line
	}
	index := strings.Index(line, prefix)
	if index < 0 {
		return line
	}
	return strings.TrimSpace(line[index:])
}

func terminalAIRiskList(value any) []string {
	raw, _ := value.(string)
	var commands []string
	if json.Unmarshal([]byte(raw), &commands) != nil {
		return nil
	}
	return commands
}

func terminalAIRisky(command string, risks []string) bool {
	command = strings.ToLower(strings.TrimSpace(command))
	for _, risk := range risks {
		risk = strings.ToLower(strings.TrimSpace(risk))
		if risk != "" && strings.Contains(command, risk) {
			return true
		}
	}
	return false
}

func validateTerminalAIPaste(command string) error {
	if strings.TrimSpace(command) == "" {
		return errors.New("empty")
	}
	for _, char := range command {
		if unicode.IsControl(char) {
			return errors.New("control")
		}
	}
	return nil
}

var terminalAIComplete = completeTerminalAICommand

func completeTerminalAICommand(prompt string) (string, error) {
	settings := loadTerminalAISettings()
	accountID, _ := settings["aiAccountId"].(string)
	account, err := terminalAIAccount(strings.TrimSpace(accountID))
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	body, _ := json.Marshal(map[string]any{
		"model": account.model,
		"messages": []map[string]string{
			{"role": "system", "content": "你是终端命令助手。只返回一条可直接执行的 shell 命令，不要解释，不要使用 markdown。"},
			{"role": "user", "content": prompt},
		},
		"temperature": 0,
		"max_tokens":  256,
	})
	endpoint := strings.TrimRight(account.baseURL, "/") + "/chat/completions"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	if account.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+account.apiKey)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("模型接口返回 %d", response.StatusCode)
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil || len(decoded.Choices) == 0 {
		return "", errors.New("模型响应缺少命令")
	}
	return cleanTerminalAICommand(decoded.Choices[0].Message.Content), nil
}

type terminalAIAccountConfig struct {
	baseURL string
	apiKey  string
	model   string
}

func terminalAIAccount(id string) (terminalAIAccountConfig, error) {
	if id == "" {
		return terminalAIAccountConfig{}, errors.New("未配置终端 AI 账号")
	}
	state := getAIState()
	state.mu.RLock()
	defer state.mu.RUnlock()
	_, account := accountByIDLocked(state.data.Accounts, id)
	if account == nil {
		return terminalAIAccountConfig{}, errors.New("终端 AI 账号不存在")
	}
	config := terminalAIAccountConfig{
		baseURL: aiString(account, "baseURL", "baseUrl"),
		apiKey:  aiString(account, "apiKey"),
		model:   aiString(account, "model"),
	}
	if config.model == "" {
		if models := accountModels(account); len(models) > 0 {
			config.model = aiString(models[0], "id", "name")
		}
	}
	if config.baseURL == "" || config.model == "" {
		return terminalAIAccountConfig{}, errors.New("终端 AI 账号缺少地址或模型")
	}
	return config, nil
}

func cleanTerminalAICommand(content string) string {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```shell")
	content = strings.TrimPrefix(content, "```sh")
	content = strings.TrimPrefix(content, "```bash")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	return strings.TrimSpace(content)
}
