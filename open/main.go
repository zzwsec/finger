package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v2"
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
	openListDir       = "open_list"

	initFileName = "init.txt"
	argsYamlName = "args.yaml"
	logFileName  = "info.log"
	pidFileName  = "open.pid"

	limitYamlFileName = "limit.yaml"
	loginYamlFileName = "login.yaml"
	openYamlFileName  = "open.yaml"
	playbookDir       = "playbook"

	registerCountSql = "select count(1) register_num from log_register where zone_id=?"
	rechargeCountSql = "select count(distinct player_id) as recharge_num from (select player_id, sum(money) as total from log_recharge where zone_id=? group by player_id having total>=6) as subquery"
)

var (
	dbHost     string
	dbPort     string
	dbUser     string
	dbPassword string
	dbName     string

	ipMap      = make(map[string][]string)
	logger     *log.Logger
	logFile    *os.File
	currentDir string
	loginIP    string
	modifyId   int
)

func main() {
	defer func() {
		if err := recover(); err != nil {
			SendMessage(fmt.Sprintf("Panic occurred: %v", err))
			os.Exit(1)
		}
	}()

	currentDir = getCurrentDir()

	// 初始化日志系统
	if err := initLogging(); err != nil {
		log.Panicf("[ERROR] 初始化日志失败: %v", err)
	}
	defer logFile.Close()

	// 前置验证
	validateGameList() // 检查 game_list.txt 文件有效性
	loadIpMap()        // 获取 game_list.txt 中 ip 和服务编号的 map

	// 加载配置
	if err := loadConfig(); err != nil {
		logger.Panicf("[ERROR] args.yaml文件加载或解析失败: %v", err)
	}

	// 单实例检查
	if checkRunning() {
		logger.Println("[INFO] 程序已在运行，退出")
		return
	}

	// 写入 PID 文件
	if err := writePID(); err != nil {
		logger.Printf("[ERROR] 写入 PID 文件失败: %v", err)
		return
	}
	defer cleanup()

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

func getCurrentDir() string {
	pwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("[ERROR]: 获取当前目录失败: %v", err)
	}
	return pwd
}

func mainLoop(db *sql.DB) {
	for {
		currentNum := getCurrentServerNum()
		nextNum := currentNum + 1

		if err := db.Ping(); err != nil {
			logger.Panicf("[ERROR] 数据库连接关闭")
		}

		registerCount, err := queryCount(db, currentNum, registerCountSql)
		if err != nil {
			logger.Panicf("[ERROR] 当前服务 %d 查询注册人数失败: %v", currentNum, err)
		}
		logger.Printf("[INFO] 当前检查注册人数 %d，服务编号为: %d", registerCount, currentNum)

		if registerCount >= 2000 {
			if handleServerSwitch(currentNum, nextNum) {
				updateServerNum(nextNum)
				continue
			} else {
				panic(fmt.Sprintf("[ERROR] 服务编号%d, 开服失败!", nextNum))
			}
		}

		rechargeCount, err := queryCount(db, currentNum, rechargeCountSql)
		if err != nil {
			logger.Panicf("[ERROR] 当前服务 %d 付费人数查询失败: %v", currentNum, err)
		}
		logger.Printf("[INFO] 当前检查付费人数 %d，服务编号为: %d", rechargeCount, currentNum)

		if rechargeCount >= 100 {
			if handleServerSwitch(currentNum, nextNum) {
				updateServerNum(nextNum)
				continue
			} else {
				panic(fmt.Sprintf("[ERROR] 服务编号%d, 开服失败!", nextNum))
			}
		}

		time.Sleep(30 * time.Second)
	}
}

func handleSignals(ch <-chan os.Signal) {
	sig := <-ch
	logger.Printf("收到信号: %v，执行清理", sig)
	cleanup()
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
		{"白名单更新", updateWhitelist, newNum},
		{"开放时间", updateOpenTime, newNum},
		{"限制名单", updateLimit, oldNum},
		{"清理日志", cleanLogs, newNum},
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

			// 指数退避等待
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

func initLogging() (err error) {
	logFile, err = os.OpenFile(filepath.Join(currentDir, logFileName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("无法打开日志文件: %v", err)
	}

	// 配置日志器
	logger = log.New(logFile, "SYS-MESSAGE ", log.Ldate|log.Ltime|log.Lshortfile)

	// 重定向标准输出
	os.Stdout = logFile
	os.Stderr = logFile

	return nil
}

func loadConfig() error {
	data, err := os.ReadFile(filepath.Join(currentDir, argsYamlName))
	if err != nil {
		return fmt.Errorf("读取配置失败: %v", err)
	}

	config := make(map[string]string)
	if err = yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析配置失败: %v", err)
	}

	// 设置配置项
	dbHost = config["dbHost"]
	dbPort = config["dbPort"]
	dbUser = config["dbUser"]
	dbPassword = config["dbPassword"]
	dbName = config["dbName"]

	return nil
}

func initDB() (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s",
		dbUser, dbPassword, dbHost, dbPort, dbName)

	var db *sql.DB
	var err error
	maxRetries := 3
	retryDelay := time.Second * 2 // 初始重试间隔

	for i := 0; i < maxRetries; i++ {
		if db != nil {
			db.Close() // 关闭之前的连接
		}

		db, err = sql.Open("mysql", dsn)
		if err != nil {
			log.Printf("[WARNING] 尝试 %d: 创建数据库对象失败: %v", i+1, err)
			continue
		}

		// 验证连接有效性
		err = db.Ping()
		if err == nil {
			break // 连接成功，退出循环
		}

		// 关闭无效连接并等待重试
		log.Printf("[WARNING] 尝试 %d 次: 数据库连接失败: %v", i+1, err)
		db.Close()
		time.Sleep(retryDelay)
		retryDelay *= 2 // 指数退避，避免雪崩
	}

	if err != nil {
		return nil, fmt.Errorf("数据库连接失败（共尝试 %d 次）: %v", maxRetries, err)
	}

	// 4. 配置连接池
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	logger.Println("[SUCCESS] 数据库连接成功")
	return db, nil
}

// #########################################动作############################################

func updateServerNum(num int) {
	if err := os.WriteFile(filepath.Join(currentDir, initFileName), []byte(strconv.Itoa(num)+"\n"), 0644); err != nil {
		logger.Panicf("[ERROR] 更新服务编号失败: %v", err)
	} else {
		logger.Printf("[SUCCESS] 已更新服务编号至 %d", num)
	}
}

func updateWhitelist(num int) error {
	loginIP, err := getLoginIP(strconv.Itoa(num))
	logger.Printf("[INFO] 正在更新白名单 服务编号为:%d 当前IP为:%s", num, loginIP)
	cmd := exec.Command("ansible", "-i", fmt.Sprintf("%s,", loginIP),
		"all",
		"-m", "shell",
		"-a", fmt.Sprintf("sed -i -e '/^%d$/d' -e '/^$/d' /data/server/login/etc/white_list.txt", num))
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
	loginIP, err = getLoginIP(strconv.Itoa(num))

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

	//logger.Printf("[INFO] 正在reload login 当前IP为:%s", loginIP)
	//cmd = exec.Command("ansible-playbook",
	//	"-i", fmt.Sprintf("%s,", loginIP),
	//	"-e", fmt.Sprintf("host_name=%s,", loginIP),
	//	filepath.Join(currentDir, playbookDir, loginYamlFileName))
	//output, err = cmd.CombinedOutput()
	//logger.Printf("[INFO] Ansible输出:\n%s", output)

	return err
}

func updateLimit(num int) error {
	loginIP, err := getLoginIP(strconv.Itoa(num))

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

// 单次执行，不处理错误
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

// ###################进程#######################

func checkRunning() bool {
	if _, err := os.Stat(filepath.Join(currentDir, pidFileName)); os.IsNotExist(err) {
		return false
	}

	data, err := os.ReadFile(filepath.Join(currentDir, pidFileName))
	if err != nil {
		logger.Printf("[WARNING] 读取PID文件失败: %v，尝试删除", err)
		os.Remove(filepath.Join(currentDir, pidFileName))
		return false
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		logger.Printf("[WARNING] 无效的PID内容: %v，删除文件", err)
		os.Remove(filepath.Join(currentDir, pidFileName))
		return false
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		logger.Printf("[WARNING] 查找进程失败: %v，删除文件", err)
		os.Remove(filepath.Join(currentDir, pidFileName))
		return false
	}

	if err := process.Signal(syscall.Signal(0)); err == nil {
		logger.Printf("[WARNING] 进程 %d 正在运行", pid)
		return true
	}

	logger.Printf("[WARNING] 进程 %d 不存在，删除PID文件", pid)
	os.Remove(filepath.Join(currentDir, pidFileName))
	return false
}

func writePID() error {
	pid := os.Getpid()
	// 使用独占模式创建文件
	file, err := os.OpenFile(filepath.Join(currentDir, pidFileName), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return fmt.Errorf("已有实例运行")
	}
	defer file.Close()

	_, err = fmt.Fprintf(file, "%d", pid)
	return err
}

func cleanup() {
	if _, err := os.Stat(filepath.Join(currentDir, pidFileName)); os.IsNotExist(err) {
		logger.Printf("[WARNING] PID 文件不存在，跳过删除")
		return
	}
	if err := os.Remove(filepath.Join(currentDir, pidFileName)); err != nil {
		logger.Printf("[WARNING] 删除PID文件失败: %v", err)
	} else {
		logger.Println("[INFO] PID文件删除成功")
	}
}

// ###############数据解析###################

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
	file, err := os.Open(filepath.Join(currentDir, openListDir, gameListFileName))
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
		if modifyId, err = strconv.Atoi(parts[2]); err != nil {
			log.Panicf("login映射无效, 检查game_list.txt")
		}
	}

	if err := scanner.Err(); err != nil {
		logger.Panicf("文件读取错误: %v", err)
	}
}

func validateGameList() {
	file, err := os.Open(filepath.Join(currentDir, openListDir, gameListFileName))
	if err != nil {
		logger.Panicf("无法打开game列表文件: %v", err)
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
	file, err := os.Open(filepath.Join(currentDir, openListDir, loginListFileName))
	if err != nil {
		logger.Panicf("无法打开 login 列表文件: %v", err)
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
	}

	if err := scanner.Err(); err != nil {
		logger.Panicf("文件读取错误: %v", err)
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

func getCurrentServerNum() int {
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

	return num
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
func getLoginIP(id string) (string, error) {
	loginPath := filepath.Join(currentDir, openListDir, loginListFileName)
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
			logger.Fatalf("[ERROR] login_list 第 %d 行字段不足: %s", lineNum, line)
		}

		if parts[1] == myid {
			logger.Printf("[INFO] 找到自定义编号 %s 对应 IP: %s", myid, parts[0])
			return parts[0], nil
		}
	}

	return "", fmt.Errorf("[ERROR] 自定义编号 %s 在 login_list.txt 中不存在", myid)
}

// 根据 game 编号从 game_list.txt 获取对应的第三列 ID
func getID(num string) (string, error) {
	gameFile, err := os.Open(filepath.Join(currentDir, openListDir, gameListFileName))
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
			if n == num {
				logger.Printf("[INFO] 找到服务编号 %s 对应 ID: %s", num, parts[2])
				return parts[2], nil // 正确返回 ID
			}
		}
	}

	return "", fmt.Errorf("[ERROR] 服务编号 %s 在 game_list.txt 中不存在", num)
}
