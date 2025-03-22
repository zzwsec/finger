package message

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

type LogRequest struct {
	Action    string `json:"action"`
	Timestamp string `json:"timestamp"`
	Country   string `json:"country"`
	OSInfo    string `json:"os_info"`
	CPUArch   string `json:"cpu_arch"`
}

func SendMessage(action string) {
	// 1. 获取国家代码
	country := getCountry()

	// 2. 获取操作系统信息
	osInfo := getOSInfo()

	// 3. 获取 CPU 架构
	cpuArch := runtime.GOARCH
	if cpuArch == "" {
		cpuArch = "unknown"
	}

	// 4. 发送请求
	sendLogRequest(LogRequest{
		Action:    action,
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		Country:   country,
		OSInfo:    osInfo,
		CPUArch:   cpuArch,
	})
}

// 国家代码获取
func getCountry() string {
	resp, err := http.Get("https://ipinfo.io/country")
	if err != nil || resp.StatusCode != 200 {
		return "unknown"
	}
	defer resp.Body.Close()

	if data, err := io.ReadAll(resp.Body); err == nil {
		return strings.TrimSpace(string(data))
	}
	return "unknown"
}

// 操作系统信息获取
func getOSInfo() string {
	// 尝试读取标准文件
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "unknown"
	}

	// 简单解析
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			return strings.Trim(strings.SplitN(line, "=", 2)[1], `"`)
		}
	}
	return "unknown"
}

// 发送请求（不处理任何错误）
func sendLogRequest(data LogRequest) {
	jsonData, _ := json.Marshal(data)
	http.Post(
		"https://api.611611.best/api/log",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
}
