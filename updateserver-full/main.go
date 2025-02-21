package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

const (
	updateListFileName = "update_list.txt"
	playbookFileName   = "example.yml"
	maxRetry           = 3
)

var (
	updateListPath    string
	updateListContent map[string][]int
	playbookPath      string
	currentDir        string
	err               error
)

func main() {
	setGlobalVar()

	// 创建一个 WaitGroup 用于等待所有 goroutine 完成
	var wg sync.WaitGroup

	// 创建一个 channel 用于接收错误信息
	errChan := make(chan error, len(updateListContent))

	// 遍历更新清单，为每个 IP 启动一个 goroutine
	for ip, svc := range updateListContent {
		wg.Add(1) // 增加 WaitGroup 的计数器
		go func(ip string, svc []int) {
			defer wg.Done() // 减少 WaitGroup 的计数器

			for i := 1; i <= maxRetry; i++ {
				log.Printf("[INFO]: 当前正在更新 %s，第 %d/%d 尝试\n", ip, i, maxRetry)
				if executeAnsible(ip, svc) {
					log.Printf("[SUCCESS]: %s 更新成功\n", ip)
					return
				} else if i == maxRetry {
					errChan <- fmt.Errorf("[ERROR]: %s 更新失败，已达到最大重试次数\n", ip)
					return
				} else {
					log.Printf("[WARNING]: %s 更新失败，重试中...\n", ip)
				}
			}
		}(ip, svc)
	}

	// 启动一个 goroutine 等待所有任务完成
	go func() {
		wg.Wait()      // 等待所有 goroutine 完成
		close(errChan) // 关闭 channel
	}()

	// 收集所有错误信息
	for err := range errChan {
		log.Println(err)
	}
}

func setGlobalVar() {
	currentDir, err = getCurrentDir()
	if err != nil {
		log.Fatalf("[ERROR]: %v", err)
	}
	updateListPath = filepath.Join(currentDir, updateListFileName)
	playbookPath = filepath.Join(currentDir, playbookFileName)
	updateListContent = parseUpdateList()
}

func parseUpdateList() map[string][]int {
	updateFile, err := os.Open(updateListPath)
	if err != nil {
		log.Fatalf("[ERROR]: 无法打开更新清单文件: %v\n", err)
	}
	defer updateFile.Close()

	tmpMap := make(map[string][]int)
	updateListBuf := bufio.NewReader(updateFile)
	for {
		tmpString, tmpErr := updateListBuf.ReadString('\n')
		if tmpErr != nil && tmpErr != io.EOF {
			log.Fatalf("[ERROR]: 读取更新清单文件失败: %v\n", tmpErr)
		}

		tmpString = strings.TrimSpace(tmpString)
		if tmpString != "" {
			ip := getIP(tmpString)
			svcList := getSvcList(tmpString)
			tmpMap[ip] = svcList
		}

		if tmpErr == io.EOF {
			break
		}
	}
	return tmpMap
}

func getIP(tmpString string) string {
	parseSlice := strings.Fields(strings.TrimSpace(tmpString))
	if len(parseSlice) < 1 {
		log.Fatalln("[ERROR]: 更新清单文件格式错误，缺少 IP 地址")
	}
	parseIP := parseSlice[0]
	re := regexp.MustCompile(`\d{1,3}(\.\d{1,3}){3}`)
	if !re.MatchString(parseIP) {
		log.Fatalf("[ERROR]: 错误的 IP 地址: %s，请检查更新清单\n", parseIP)
	}
	return parseIP
}

func getSvcList(tmpString string) []int {
	parseSlice := strings.Fields(strings.TrimSpace(tmpString))
	if len(parseSlice) < 2 {
		log.Fatalln("[ERROR]: 更新清单文件格式错误，缺少服务编号")
	}
	parseSvc := parseSlice[1]
	re := regexp.MustCompile(`\d+`)
	tmpSliceString := re.FindAllString(parseSvc, -1)
	tmpSliceInt := make([]int, 0)
	for _, i := range tmpSliceString {
		toInt, err := strconv.Atoi(i)
		if err != nil {
			log.Printf("[WARNING]: 错误的服务编号: %s，跳过\n", i)
			continue
		}
		tmpSliceInt = append(tmpSliceInt, toInt)
	}
	return tmpSliceInt
}

func getCurrentDir() (string, error) {
	pwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return pwd, nil
}

func executeAnsible(ip string, svc []int) bool {
	logDir := filepath.Join(currentDir, "logs")
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		os.Mkdir(logDir, 0755)
	}

	file, err := os.Create(filepath.Join(currentDir, "logs", ip+".log"))
	if err != nil {
		log.Printf("[WARNING]: ansible日志创建失败: %v\n", err)
		return false
	}
	defer file.Close()

	gameLoopJSON, err := json.Marshal(svc)
	if err != nil {
		log.Printf("[ERROR]: 转换 JSON 失败: %v\n", err)
		return false
	}

	extraVars := fmt.Sprintf(`{"game_servers": %s}`, string(gameLoopJSON))

	cmd := exec.Command("ansible-playbook", playbookPath,
		"-i", fmt.Sprintf("%s,", ip),
		"-e", fmt.Sprintf("host_name=%s", ip),
		"-e", extraVars,
	)

	cmd.Stdout = file
	cmd.Stderr = file

	if err := cmd.Run(); err != nil {
		log.Printf("[ERROR]: 执行 ansible-playbook 失败: %v\n", err)
		return false
	}

	return true
}
