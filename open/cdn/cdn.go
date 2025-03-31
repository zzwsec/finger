package cdn

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"open/loglevel"
	"strconv"
	"time"
)

var successLogger = loglevel.GetSuccessLogger()

type RequestData struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func FlushCDN(num int, cdnURL string) error {
	params := url.Values{
		"zone_id": []string{strconv.Itoa(num)},
	}
	fullURL := cdnURL + "?" + params.Encode()

	client := &http.Client{Timeout: 5 * time.Second} // 5s 超时
	resp, err := client.Get(fullURL)
	if err != nil {
		return fmt.Errorf("HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP 状态码错误: %d", resp.StatusCode)
	}

	var reqData RequestData
	if err = json.NewDecoder(resp.Body).Decode(&reqData); err != nil {
		return fmt.Errorf("解析响应体失败: %w", err)
	}

	if reqData.Code != 0 {
		return fmt.Errorf("返回状态非 0，Message: %s", reqData.Message)
	}

	successLogger.Printf("CDN 刷新成功，zone_id: %d", num)
	return nil
}
