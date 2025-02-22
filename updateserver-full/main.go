package main

import (
	"bufio"
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
	"time"
)

const (
	gameListFileName  = "game_list.txt"
	gateListFileName  = "gate_list.txt"
	loginListFileName = "login_list.txt"

	baseYamlFileName  = "base.yaml"
	gameYamlFileName  = "game-entry.yaml"
	gateYamlFileName  = "gate.yaml"
	loginYamlFileName = "login.yaml"

	updateListDirName = "update-list"
	playbookDirName   = "playbook"

	maxRetry = 3
)

var (
	currentPath    string
	updateListPath string
	playbookPath   string

	gameListContent  map[string][]int
	gateListContent  []string
	loginListContent []string
	baseIPList       []string

	wg      sync.WaitGroup
	errChan chan error
)

func main() {
	setGlobalVar()

	errChan = make(chan error, 1)
	defer close(errChan)

	go func() {
		if err := <-errChan; err != nil {
			log.Fatalf("[FATAL] 关键错误: %v", err) // 直接终止程序
		}
	}()

	for _, ip := range gateListContent {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			if !executeGateAndLoginAnsible(gateYamlFileName, ip, "gate-stop") {
				// 非阻塞式发送错误
				select {
				case errChan <- fmt.Errorf("gate停止失败: %s", ip):
				default:
				}
			}
		}(ip)
	}
	wg.Wait()

	for _, ip := range loginListContent {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			if !executeGateAndLoginAnsible(loginYamlFileName, ip, "login-stop") {
				select {
				case errChan <- fmt.Errorf("login停止失败: %s", ip):
				default:
				}
			}
		}(ip)
	}
	wg.Wait()

	for _, ip := range baseIPList {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			if !executeBaseAnsible(ip) {
				select {
				case errChan <- fmt.Errorf("base任务失败: %s", ip):
				default:
				}
			}
		}(ip)
	}
	wg.Wait()

	for ip, svcs := range gameListContent {
		wg.Add(1)
		go func(ip string, svcs []int) {
			defer wg.Done()
			if !executeGameAnsible(gameYamlFileName, ip, svcs) {
				select {
				case errChan <- fmt.Errorf("game停止失败: %s", ip):
				default:
				}
			}
		}(ip, svcs)
	}
	wg.Wait()

	for _, ip := range gateListContent {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			if !executeGateAndLoginAnsible(gateYamlFileName, ip, "gate-start") {
				select {
				case errChan <- fmt.Errorf("gate启动失败: %s", ip):
				default:
				}
			}
		}(ip)
	}
	wg.Wait()

	for _, ip := range loginListContent {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			if !executeGateAndLoginAnsible(loginYamlFileName, ip, "login-start") {
				select {
				case errChan <- fmt.Errorf("login启动失败: %s", ip):
				default:
				}
			}
		}(ip)
	}
	wg.Wait()
}

func setGlobalVar() {
	currentPath = getCurrentDir()
	updateListPath = filepath.Join(currentPath, updateListDirName)
	playbookPath = filepath.Join(currentPath, playbookDirName)
	gameListContent = parseGameListFile()
	gateListContent = parseListFile(gateListFileName)
	loginListContent = parseListFile(loginListFileName)
	baseIPList = getAllUniqueIPs()
}

func parseListFile(fileName string) []string {
	listFile, err := os.Open(filepath.Join(updateListPath, fileName))
	if err != nil {
		log.Fatalf("[ERROR]: 无法打开%s更新清单文件: %v\n", fileName, err)
	}
	defer listFile.Close()

	tmpSlice := make([]string, 0)
	listBuf := bufio.NewReader(listFile)
	for {
		tmpString, tmpErr := listBuf.ReadString('\n')
		if tmpErr != nil && tmpErr != io.EOF {
			log.Fatalf("[ERROR]: 读取%s更新清单文件失败: %v\n", fileName, tmpErr)
		}

		tmpString = strings.TrimSpace(tmpString)
		if tmpString != "" {
			ip := getIP(tmpString)
			tmpSlice = append(tmpSlice, ip)
		}

		if tmpErr == io.EOF {
			break
		}
	}
	return tmpSlice
}

func parseGameListFile() map[string][]int {
	gameListFile, err := os.Open(filepath.Join(updateListPath, gameListFileName))
	if err != nil {
		log.Fatalf("[ERROR]: 无法打开game更新清单文件: %v\n", err)
	}
	defer gameListFile.Close()

	tmpMap := make(map[string][]int)
	gameListBuf := bufio.NewReader(gameListFile)
	for {
		tmpString, tmpErr := gameListBuf.ReadString('\n')
		if tmpErr != nil && tmpErr != io.EOF {
			log.Fatalf("[ERROR]: 读取更新清单文件失败: %v\n", tmpErr)
		}

		tmpString = strings.TrimSpace(tmpString)
		if tmpString != "" {
			fields := strings.Fields(tmpString)
			if len(fields) < 2 {
				log.Fatalf("[ERROR]: 格式错误的行: %s\n", tmpString)
			}
			ip := getIP(fields[0])
			svcList := getGameList(fields[1])
			tmpMap[ip] = svcList
		}

		if tmpErr == io.EOF {
			break
		}
	}
	return tmpMap
}

func getIP(ipStr string) string {
	parseIP := strings.TrimSpace(ipStr)
	re := regexp.MustCompile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`)
	if !re.MatchString(parseIP) {
		log.Fatalf("[ERROR]: 错误的 IP 地址: %s\n", parseIP)
	}
	return parseIP
}

func getGameList(svcListStr string) []int {
	parseSvc := strings.Trim(svcListStr, "[] \t\n\r")
	if parseSvc == "" {
		return []int{}
	}

	tmpSliceString := strings.Split(parseSvc, ",")
	tmpSliceInt := make([]int, 0)

	for _, svcStr := range tmpSliceString {
		svcStr = strings.TrimSpace(svcStr)
		if svcStr == "" {
			continue
		}

		svcInt, err := strconv.Atoi(svcStr)
		if err != nil {
			log.Fatalf("[ERROR]: 错误的服务编号: %s\n", svcStr)
		}
		tmpSliceInt = append(tmpSliceInt, svcInt)
	}

	return tmpSliceInt
}

func getCurrentDir() string {
	pwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("[ERROR]: 获取当前目录失败: %v", err)
	}
	return pwd
}

func executeGateAndLoginAnsible(bookName, ip, tag string) bool {
	log.Printf("[INFO]: 开始执行 %s 的 %s 任务", ip, tag)
	for i := 1; i <= maxRetry; i++ {
		file, ok := openLogFile(ip)
		bookPath := filepath.Join(playbookPath, bookName)
		cmd := exec.Command("ansible-playbook", bookPath,
			"-i", fmt.Sprintf("%s,", ip),
			"--flush-cache",
			"--timeout", "60",
			"-e", fmt.Sprintf("host_name=%s", ip),
			"-t", tag,
		)

		if ok {
			defer file.Close()
			cmd.Stdout = file
			cmd.Stderr = file
		} else {
			cmd.Stdout = nil
			cmd.Stderr = nil
		}
		start := time.Now()
		err := cmd.Run()
		if err != nil {
			log.Printf("[ERROR]: 执行 %s 下的 %s 任务失败，耗时: %s\n", ip, bookName, time.Since(start))
			time.Sleep(time.Duration(i*i) * time.Second)
			continue
		}

		log.Printf("[SUCCESS]: 执行 %s 下的 %s 任务成功，耗时: %s\n", ip, bookName, time.Since(start))
		return true
	}
	return false
}

func executeBaseAnsible(ip string) bool {
	log.Printf("[INFO]: 正在分发文件, 当前ip: %s\n", ip)
	for i := 1; i <= maxRetry; i++ {
		file, ok := openLogFile(ip)
		bookPath := filepath.Join(playbookPath, baseYamlFileName)
		cmd := exec.Command("ansible-playbook", bookPath,
			"-i", fmt.Sprintf("%s,", ip),
			"--flush-cache",
			"--timeout", "60",
			"-e", fmt.Sprintf("host_name=%s", ip),
		)
		if ok {
			defer file.Close()
			cmd.Stdout = file
			cmd.Stderr = file
		} else {
			cmd.Stdout = nil
			cmd.Stderr = nil
		}
		start := time.Now()
		if err := cmd.Run(); err != nil {
			log.Printf("[ERROR]: 执行 %s 下的 %s 任务失败，耗时: %s\n", ip, baseYamlFileName, time.Since(start))
			time.Sleep(time.Duration(i*i) * time.Second)
			continue
		}

		log.Printf("[SUCCESS]: 执行 %s 下的 %s 任务成功，耗时: %s\n", ip, baseYamlFileName, time.Since(start))
		return true
	}
	return false
}

func executeGameAnsible(bookName, ip string, svcs []int) bool {
	log.Printf("[INFO]: 开始执行 %s 的游戏更新任务，服务编号: %v", ip, svcs)
	for i := 1; i <= maxRetry; i++ {
		file, ok := openLogFile(ip)
		bookPath := filepath.Join(playbookPath, bookName)
		extraVars := makeJson(svcs)

		cmd := exec.Command("ansible-playbook", bookPath,
			"-i", fmt.Sprintf("%s,", ip),
			"--flush-cache",
			"--timeout", "600",
			"-e", fmt.Sprintf("host_name=%s", ip),
			"-e", extraVars,
		)
		if ok {
			defer file.Close()
			cmd.Stdout = file
			cmd.Stderr = file
		} else {
			cmd.Stdout = nil
			cmd.Stderr = nil
		}
		start := time.Now()
		if err := cmd.Run(); err != nil {
			log.Printf("[ERROR]: 执行 %s 下的 %s 任务失败，耗时: %s\n", ip, bookName, time.Since(start))
			time.Sleep(time.Duration(i*i) * time.Second)
			continue
		}

		log.Printf("[SUCCESS]: 执行 %s 下的 %s 任务成功，耗时: %s\n", ip, bookName, time.Since(start))
		return true
	}
	return false
}

func openLogFile(ip string) (*os.File, bool) {
	logDir := filepath.Join(currentPath, "logs")
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		os.Mkdir(logDir, 0755)
	}
	logPath := filepath.Join(logDir, ip+".log")
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("[WARNING]: %s.log 日志创建失败: %v\n", ip, err)
		return nil, false
	}
	return file, true
}

func makeJson(svcs []int) string {
	if len(svcs) == 0 {
		return `{"game_servers": []}`
	}

	var tmpList strings.Builder
	for i, svc := range svcs {
		if i > 0 {
			tmpList.WriteString(",")
		}
		tmpList.WriteString(fmt.Sprintf("%d", svc))
	}
	return fmt.Sprintf(`{"game_servers": [%s]}`, tmpList.String())
}

func getAllUniqueIPs() []string {
	// 合并所有IP地址到临时切片
	allIPs := make([]string, 0)
	allIPs = append(allIPs, gateListContent...)
	allIPs = append(allIPs, loginListContent...)
	allIPs = append(allIPs, getGameIPs()...)

	// 使用map实现去重
	ipMap := make(map[string]struct{})
	for _, ip := range allIPs {
		ipMap[ip] = struct{}{} // 空结构体不占用内存空间
	}

	// 转换回有序切片
	uniqueIPs := make([]string, 0, len(ipMap))
	for ip := range ipMap {
		uniqueIPs = append(uniqueIPs, ip)
	}
	return uniqueIPs
}

func getGameIPs() []string {
	ips := make([]string, 0, len(gameListContent))
	for ip := range gameListContent {
		ips = append(ips, ip)
	}
	return ips
}
