package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	loginListFileName = "login_list.txt"
	gameListFileName  = "game_list.txt"

	initFileName = "init.txt"

	limitYamlFileName = "limit.yaml"
	loginYamlFileName = "login.yaml"
	openYamlFileName  = "open.yaml"
	playbookDir       = "playbook"

	whiteListPath = "/data/server/white-limit/white_list.txt"

	registerCountSql = "select count(1) register_num from log_register where zone_id=?"
	rechargeCountSql = "select count(distinct player_id) as recharge_num from (select player_id, sum(money) as total from log_recharge where zone_id=? group by player_id having total>=6) as subquery"
)

var (
	dbHost                string
	dbPort                string
	dbUser                string
	dbPassword            string
	dbName                string
	criticalRegisterCount int
	criticalRechargeCount int
	sleepInterval         int

	ipMap      = make(map[string][]string)
	logger     *log.Logger
	currentDir string
	currentNum int
)

func main() {
	defer func() {
		if err := recover(); err != nil {
			SendMessage(fmt.Sprintf("Panic occurred: %v", err))
			os.Exit(1)
		}
	}()

	currentDir = getCurrentDir()

	validateGameList()    // 验证 game_list.txt 文件
	validateLoginList()   // 验证 login_list.txt 文件
	loadIpMap()           // 配置 map
	getCurrentServerNum() // 获取当前服务编号

	// 加载配置
	if err := loadEnv(); err != nil {
		logger.Panicf("[ERROR] 环境变量解析失败: %v", err)
	}

	// 信号处理
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go handleSignals(sigCh)

	// 数据库连接
	db, err := initDB()
	if err != nil {
		logger.Panicf("[ERROR] 数据库初始化失败: %v", err)
	}
	defer db.Close()
	mainLoop(db)
}

func mainLoop(db *sql.DB) {
	for {
		if err := db.Ping(); err != nil {
			logger.Printf("[ERROR] 数据库连接失效，尝试重连")
			db, err = initDB()
			if err != nil {
				logger.Printf("[ERROR] 数据库重连失败: %v", err)
				time.Sleep(time.Duration(sleepInterval) * time.Second)
				continue
			}
		}

		registerCount, err := queryCount(db, currentNum, registerCountSql)
		if err != nil {
			logger.Printf("[ERROR] 查询注册人数失败: %v", err)
			time.Sleep(time.Duration(sleepInterval) * time.Second)
			continue
		}
		logger.Printf("[INFO] 当前注册人数 %d，服务编号 %d", registerCount, currentNum)

		nextNum := currentNum + 1
		if registerCount >= criticalRegisterCount {
			if handleServerSwitch(currentNum, nextNum) {
				updateServerNum(nextNum)
			} else {
				logger.Printf("[ERROR] 服务编号 %d 开服失败", nextNum)
			}
			continue
		}

		rechargeCount, err := queryCount(db, currentNum, rechargeCountSql)
		if err != nil {
			logger.Printf("[ERROR] 查询付费人数失败: %v", err)
			time.Sleep(time.Duration(sleepInterval) * time.Second)
			continue
		}
		logger.Printf("[INFO] 当前付费人数 %d，服务编号 %d", rechargeCount, currentNum)

		if rechargeCount >= criticalRechargeCount {
			if handleServerSwitch(currentNum, nextNum) {
				updateServerNum(nextNum)
			} else {
				logger.Printf("[ERROR] 服务编号 %d 开服失败", nextNum)
			}
			continue
		}

		time.Sleep(time.Duration(30) * time.Second)
	}
}

func getCurrentDir() string {
	pwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("[ERROR]: 获取当前目录失败: %v", err)
	}
	return pwd
}

func handleSignals(ch <-chan os.Signal) {
	sig := <-ch
	logger.Printf("[INFO] 收到信号: %v，执行清理", sig)
	if db != nil {
		db.Close()
	}
	SendMessage("用户手动退出")
	os.Exit(0)
}
func handleServerSwitch(oldNum, newNum int) bool {
	if !validateNextServer(newNum) {
		return false
	}

	ops := []struct {
		name string
		fn   func(int) error
		arg  int
	}{
		{"清理日志", cleanLogs, newNum},
		{"开服时间", updateOpenTime, newNum},
		{"白名单更新", updateWhitelist, newNum},
		{"休眠间隔", updateSleepTime, sleepInterval},
		{"限制名单", updateLimit, oldNum},
	}

	for _, op := range ops {
		if !executeWithRetry(op.name, op.fn, op.arg) {
			return false
		}
	}
	return true
}

func executeWithRetry(opName string, fn func(int) error, arg int) bool {
	const maxRetries = 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		logger.Printf("[INFO] 执行 %s (尝试 %d/%d)", opName, attempt, maxRetries)

		if err := fn(arg); err != nil {
			lastErr = err
			logger.Printf("[WARNING] %s 尝试 %d 失败: %v", opName, attempt, err)

			// 等待
			if attempt < maxRetries {
				logger.Printf("[WARNING] %s 等待 %v 后重试...", opName, time.Duration(attempt*attempt)*time.Second)
				time.Sleep(time.Duration(attempt*attempt) * time.Second)
			}
			continue
		}

		logger.Printf("[SUCCESS] %s 成功", opName)
		return true
	}

	logger.Printf("[ERROR] %s 失败 (共尝试 %d 次)，最后错误: %v", opName, maxRetries, lastErr)
	return false
}

func loadEnv() error {
	var err error
	dbHost = os.Getenv("dbHost")
	dbPort = os.Getenv("dbPort")
	dbUser = os.Getenv("dbUser")
	dbPassword = os.Getenv("dbPassword")
	dbName = os.Getenv("dbName")

	if dbHost == "" || dbPort == "" || dbUser == "" || dbPassword == "" || dbName == "" {
		return errors.New("数据库配置不齐全，请检查环境变量")
	}
	if net.ParseIP(dbHost) == nil {
		return errors.New("数据库IP地址格式不正确")
	}
	if _, err = strconv.Atoi(dbPort); err != nil {
		return errors.New("数据库端口非数字")
	}

	criticalRegisterCount, err = strconv.Atoi(os.Getenv("criticalRegisterCount"))
	if err != nil || criticalRegisterCount <= 0 {
		return errors.New("注册人数临界值无效")
	}
	criticalRechargeCount, err = strconv.Atoi(os.Getenv("criticalRechargeCount"))
	if err != nil || criticalRechargeCount <= 0 {
		return errors.New("付费人数临界值无效")
	}
	sleepInterval, err = strconv.Atoi(os.Getenv("sleepInterval"))
	if err != nil || sleepInterval < 0 {
		return errors.New("休眠间隔无效")
	}
	return nil
}

func initDB() (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", dbUser, dbPassword, dbHost, dbPort, dbName)
	var db *sql.DB
	var err error
	maxRetries := 3
	retryDelay := time.Second * 2

	for i := 0; i < maxRetries; i++ {
		db, err = sql.Open("mysql", dsn)
		if err != nil {
			logger.Printf("[WARNING] 尝试 %d: 创建数据库对象失败: %v", i+1, err)
			time.Sleep(retryDelay)
			retryDelay *= 2
			continue
		}
		if err = db.Ping(); err == nil {
			break
		}
		logger.Printf("[WARNING] 尝试 %d: 数据库连接失败: %v", i+1, err)
		if db != nil {
			db.Close()
		}
		db = nil
		time.Sleep(retryDelay)
		retryDelay *= 2
	}
	if db == nil {
		return nil, fmt.Errorf("数据库连接失败（共尝试 %d 次）: %v", maxRetries, err)
	}

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)
	logger.Println("[SUCCESS] 数据库连接成功")
	return db, nil
}

// #########################################动作############################################
func cleanLogs(num int) error {
	ip, err := getServerIP(num)
	if err != nil {
		return err
	}

	logger.Printf("[INFO] 正在清理日志 服务:%d IP:%s", num, ip)
	cmd := exec.Command("ansible", "-i", ip+",", "all",
		"-m", "shell",
		"-a", fmt.Sprintf("rm -rfv /data/server%d/game/log/*", num))

	output, err := cmd.CombinedOutput()
	logger.Printf("[INFO] Ansible输出:\n%s", output)

	return err
}

func updateSleepTime(i int) error {
	time.Sleep(time.Duration(i) * time.Minute)
	return nil
}

func updateServerNum(num int) {
	if err := os.WriteFile(filepath.Join(currentDir, initFileName), []byte(strconv.Itoa(num)+"\n"), 0644); err != nil {
		logger.Panicf("[ERROR] 更新服务编号失败: %v", err)
	} else {
		logger.Printf("[SUCCESS] 已更新服务编号至 %d", num)
	}
}

func updateWhitelist(num int) error {
	loginIP, err := getLoginIP(num)
	logger.Printf("[INFO] 正在更新白名单 服务编号为:%d 当前IP为:%s", num, loginIP)
	cmd := exec.Command("ansible", "-i", fmt.Sprintf("%s,", loginIP),
		"all",
		"-m", "shell",
		"-a", fmt.Sprintf("sed -i -e '/^%d$/d' -e '/^$/d' %s", num, whiteListPath))
	output, err := cmd.CombinedOutput()
	logger.Printf("[INFO] Ansible输出:\n%s", output)
	if err != nil {
		log.Fatalf("[ERROR] 命令执行失败: %v", err)
	}

	logger.Printf("[INFO] 正在reload login 当前IP为:%s", loginIP)
	cmd = exec.Command("ansible-playbook",
		"-i", fmt.Sprintf("%s,", loginIP),
		"-e", fmt.Sprintf("host_name=%s,", loginIP),
		filepath.Join(currentDir, playbookDir, loginYamlFileName))
	output, err = cmd.CombinedOutput()
	logger.Printf("[INFO] Ansible输出:\n%s", output)

	return err
}

func updateOpenTime(num int) error {
	ip, err := getServerIP(num)
	if err != nil {
		return err
	}

	logger.Printf("[INFO] 正在设置开放时间 服务编号为:%d 当前IP为:%s", num, ip)
	cmd := exec.Command("ansible-playbook", "-i", ip+",",
		"-e", "host_name="+ip,
		"-e", "svc_num="+strconv.Itoa(num),
		filepath.Join(currentDir, playbookDir, openYamlFileName))

	output, err := cmd.CombinedOutput()
	logger.Printf("[INFO] Ansible输出:\n%s", output)

	if err != nil {
		log.Fatalf("[ERROR] 命令执行失败: %v", err)
	}

	return err
}

func updateLimit(num int) error {
	loginIP, err := getLoginIP(num)

	logger.Printf("[INFO] 正在更新限制名单 服务:%d IP:%s", num, loginIP)
	cmd := exec.Command("ansible-playbook", "-i", fmt.Sprintf("%s,", loginIP),
		"-e", fmt.Sprintf("host_name=%s,", loginIP),
		"-e", fmt.Sprintf("svc_num=%d", num),
		filepath.Join(currentDir, playbookDir, limitYamlFileName))

	output, err := cmd.CombinedOutput()
	logger.Printf("[INFO] Ansible输出:\n%s", output)

	if err != nil {
		log.Fatalf("[ERROR] 命令执行失败: %v", err)
	}
	logger.Printf("[INFO] 正在reload login 当前IP为:%s", loginIP)
	cmd = exec.Command("ansible-playbook",
		"-i", fmt.Sprintf("%s,", loginIP),
		"-e", fmt.Sprintf("host_name=%s,", loginIP),
		filepath.Join(currentDir, playbookDir, loginYamlFileName))
	output, err = cmd.CombinedOutput()
	logger.Printf("[INFO] Ansible输出:\n%s", output)

	return err
}

// ########################消息############################

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

	// 4. 发送请求（不关心结果）
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
		"https://api.honeok.com/api/log",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
}

// ##########################获取##################################
// 通过服务编号获取ip
func getServerIP(num int) (string, error) {
	for ip, nums := range ipMap {
		for _, n := range nums {
			if strconv.Itoa(num) == n {
				return ip, nil
			}
		}
	}
	return "", fmt.Errorf("未找到服务编号 %d 对应的IP", num)
}

func loadIpMap() {
	file, err := os.Open(filepath.Join(currentDir, gameListFileName))
	if err != nil {
		logger.Panicf("无法打开列表文件: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		tmpString := strings.TrimSpace(line)
		if tmpString == "" {
			continue
		}
		parts := strings.Fields(line)

		ip := parts[0]
		numbers := strings.Split(parts[1][1:len(parts[1])-1], ",")
		ipMap[ip] = numbers
	}

	if err := scanner.Err(); err != nil {
		logger.Panicf("文件读取错误: %v", err)
	}
}

func validateGameList() {
	file, err := os.Open(filepath.Join(currentDir, gameListFileName))
	if err != nil {
		logger.Panicf("无法打开 game 列表文件: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// 跳过空行
		tmpString := strings.TrimSpace(line)
		if tmpString == "" {
			continue
		}

		parts := strings.Fields(line)
		if net.ParseIP(parts[0]) == nil {
			logger.Panicf("第%d行包含无效IP: %s", lineNum, parts[0])
		}

		if !strings.HasPrefix(parts[1], "[") || !strings.HasSuffix(parts[1], "]") {
			logger.Panicf("第%d行服务编号格式错误", lineNum)
		}

		nums := strings.Split(parts[1][1:len(parts[1])-1], ",")
		for _, n := range nums {
			if _, err := strconv.Atoi(n); err != nil {
				logger.Panicf("第%d行服务编号包含无效数字: %s", lineNum, n)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		logger.Panicf("文件读取错误: %v", err)
	}
}

func validateLoginList() {
	loginListPath := filepath.Join(currentDir, loginListFileName)
	file, err := os.Open(loginListPath)
	if err != nil {
		log.Fatalf("无法打开 login_list.txt 文件: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	loginValues := make(map[string]bool)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			log.Fatalf("login_list.txt 格式错误: %s", line)
		}

		if net.ParseIP(fields[0]) == nil {
			logger.Panicf("login_list.txt 包含无效IP: %s", fields[0])
		}

		loginValue := fields[1]
		if loginValues[loginValue] {
			// 如果能成功取出值，说明有重复的键
			log.Fatalf("重复的login值: %s", loginValue)
		}
		loginValues[loginValue] = true
	}
}

func validateNextServer(num int) bool {
	_, err := getServerIP(num)
	if err != nil {
		logger.Printf("无效的下个服务编号 %d: %v", num, err)
		return false
	}
	return true
}

func getCurrentServerNum() {
	file, err := os.Open(filepath.Join(currentDir, initFileName))
	if err != nil {
		logger.Panicf("打开init文件失败: %v", err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		logger.Panicf("读取init文件失败: %v", err)
	}

	num, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		logger.Panicf("无效的服务编号: %v", err)
	}

	currentNum = num
}

func queryCount(db *sql.DB, zoneID int, querySql string) (int, error) {
	var count int
	err := db.QueryRow(querySql, zoneID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("数据库查询错误: %v", err)
	}
	return count, nil
}

// 根据服务编号从 login_list.txt 获取 IP
func getLoginIP(id int) (string, error) {
	loginPath := filepath.Join(currentDir, loginListFileName)
	loginFile, err := os.Open(loginPath)
	if err != nil {
		return "", fmt.Errorf("[ERROR] 打开 login_list.txt 失败: %w", err)
	}
	defer loginFile.Close()

	myid, err := getID(id)
	if err != nil {
		log.Fatalf("[ERROR] 未从game_list.txt中找到有效的login映射编号：%v", err)
	}

	scanner := bufio.NewScanner(loginFile)
	for lineNum := 1; scanner.Scan(); lineNum++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		// 格式校验
		if len(parts) < 2 {
			logger.Fatalf("[ERROR] login_list 第 %d 行列数不足两列: %s", lineNum, line)
		}

		if parts[1] == myid {
			logger.Printf("[INFO] 找到自定义编号 %s 对应 IP: %s", myid, parts[0])
			return parts[0], nil
		}
	}

	return "", fmt.Errorf("[ERROR] 自定义编号 %s 在 login_list.txt 中不存在", myid)
}

// 根据 game 编号从 game_list.txt 获取对应的第三列 ID
func getID(num int) (string, error) {
	gameFile, err := os.Open(filepath.Join(currentDir, gameListFileName))
	if err != nil {
		return "", fmt.Errorf("[ERROR] 打开 game_list.txt 失败: %w", err)
	}
	defer gameFile.Close()

	scanner := bufio.NewScanner(gameFile)
	for lineNum := 1; scanner.Scan(); lineNum++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		numList := parts[1]

		// 匹配目标编号
		nums := strings.Split(numList[1:len(numList)-1], ",")
		for _, n := range nums {
			if n == strconv.Itoa(num) {
				logger.Printf("[INFO] 找到服务编号 %d 对应 ID: %s", num, parts[2])
				return parts[2], nil // 正确返回 ID
			}
		}
	}

	return "", fmt.Errorf("[ERROR] 服务编号 %d 在 game_list.txt 中不存在", num)
}
