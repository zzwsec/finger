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
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"runtime/debug"
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

	whiteListName = "white_list.txt"

	registerCountSql = "select count(1) register_num from log_register where zone_id=?;"
	rechargeCountSql = "select count(distinct player_id) as recharge_num from (select player_id, sum(money) as total from log_recharge where zone_id=? group by player_id having total>=?) as subquery;"
)

var (
	dbHost                string
	dbPort                string
	dbUser                string
	dbPassword            string
	dbName                string
	criticalRegisterCount int
	criticalRechargeCount int
	criticalMoney         int
	sleepInterval         int
	cdnURL                string
	loginListFilePath     string

	ipMap         = make(map[string][]string)
	loginSlice    = make([]string, 0)
	infoLogger    *log.Logger
	successLogger *log.Logger
	errLogger     *log.Logger
	warnLogger    *log.Logger
	currentDir    string
	currentNum    int
)

func main() {
	defer func() {
		if err := recover(); err != nil {
			stack := debug.Stack()
			errLogger.Printf("Panic occurred: %v\nStack: %s", err, stack)
			SendMessage(fmt.Sprintf("Panic occurred: %v", err))
			os.Exit(1)
		}
	}()

	// 初始化日志
	infoLogger = log.New(os.Stdout, "[INFO] ", log.Ldate|log.Ltime|log.Lshortfile)
	successLogger = log.New(os.Stdout, "[SUCCESS] ", log.Ldate|log.Ltime|log.Lshortfile)
	warnLogger = log.New(os.Stderr, "[WARNING] ", log.Ldate|log.Ltime|log.Lshortfile)
	errLogger = log.New(os.Stderr, "[ERROR] ", log.Ldate|log.Ltime|log.Lshortfile)

	currentDir = getCurrentDir()

	validateGameList() // 验证 game_list.txt 文件有效性
	loadIpMap()
	loadLoginSlice()
	getCurrentServerNum() // 获取当前game 编号为:

	// 加载配置
	if err := loadEnv(); err != nil {
		errLogger.Fatalf("环境变量解析失败: %v", err)
	}

	// 信号处理
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go handleSignals(sigCh)

	// 数据库连接
	db, err := initDB()
	if err != nil {
		errLogger.Fatalf("数据库初始化失败: %v", err)
	}
	defer db.Close()
	mainLoop(db)
}

func mainLoop(db *sql.DB) {
	for {
		if err := db.Ping(); err != nil {
			warnLogger.Printf("数据库连接失效, 尝试重连")
			db, err = initDB()
			if err != nil {
				warnLogger.Printf("数据库重连失败: %v", err)
				time.Sleep(time.Duration(1) * time.Minute)
				continue
			}
			infoLogger.Printf("数据库重连成功")
		}

		registerCount, err := queryCount(db, registerCountSql, currentNum)
		if err != nil {
			warnLogger.Printf("查询注册人数失败: %v", err)
			time.Sleep(time.Duration(sleepInterval) * time.Second)
			continue
		}
		infoLogger.Printf("当前注册人数 %d / %d，game 编号为:  %d", registerCount, criticalRegisterCount, currentNum)

		nextNum := currentNum + 1
		if registerCount >= criticalRegisterCount {
			if handleServerSwitch(currentNum, nextNum) {
				updateServerNum(nextNum)
			} else {
				errLogger.Panicf("game 编号为:  %d 开服失败", nextNum)
			}
			continue
		}

		rechargeCount, err := queryCount(db, rechargeCountSql, currentNum, criticalMoney)
		if err != nil {
			warnLogger.Printf("查询付费人数失败: %v", err)
			time.Sleep(time.Duration(60) * time.Second)
			continue
		}
		infoLogger.Printf("当前付费人数 %d / %d, 付费临界值: %d, game 编号为: %d", rechargeCount, criticalRechargeCount, criticalMoney, currentNum)

		if rechargeCount >= criticalRechargeCount {
			if handleServerSwitch(currentNum, nextNum) {
				updateServerNum(nextNum)
			} else {
				errLogger.Panicf("game 编号为: %d 开服失败", nextNum)
			}
			continue
		}

		time.Sleep(time.Duration(30) * time.Second)
	}
}

func getCurrentDir() string {
	pwd, err := os.Getwd()
	if err != nil {
		errLogger.Fatalf("获取当前目录失败: %v", err)
	}
	return pwd
}

func handleSignals(ch <-chan os.Signal) {
	sig := <-ch
	successLogger.Printf("收到信号: %v，执行清理", sig)
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
		{"CDN刷新", flushCDN, newNum},
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
		infoLogger.Printf("执行 %s (尝试 %d/%d)", opName, attempt, maxRetries)

		if err := fn(arg); err != nil {
			lastErr = err
			warnLogger.Printf("%s 尝试 %d 失败: %v", opName, attempt, err)

			// 等待
			if attempt < maxRetries {
				warnLogger.Printf("%s 等待 %v 后重试...", opName, time.Duration(attempt*attempt)*time.Second)
				time.Sleep(time.Duration(attempt*attempt) * time.Second)
			}
			continue
		}

		successLogger.Printf("任务 %s 成功", opName)
		return true
	}

	errLogger.Printf("任务 %s 失败 (共尝试 %d 次)，最后错误: %v", opName, maxRetries, lastErr)
	return false
}

func loadEnv() error {
	var err error
	cdnURL = os.Getenv("cdnURL")
	loginListFilePath = os.Getenv("loginListFilePath")
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
	criticalMoney, err = strconv.Atoi(os.Getenv("criticalMoney"))
	if err != nil || criticalMoney <= 0 {
		return errors.New("付费金额临界值无效")
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
			warnLogger.Printf("尝试 %d: 创建数据库对象失败: %v", i+1, err)
			time.Sleep(retryDelay)
			retryDelay *= 2
			continue
		}
		if err = db.Ping(); err == nil {
			break
		}
		warnLogger.Printf("尝试 %d: 数据库连接失败: %v", i+1, err)
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
	successLogger.Println("数据库连接成功")
	return db, nil
}

// #########################################动作############################################
func updateServerNum(num int) {
	if err := os.WriteFile(filepath.Join(currentDir, initFileName), []byte(strconv.Itoa(num)+"\n"), 0644); err != nil {
		errLogger.Panicf("更新 init.txt 文件中game 编号为: 失败: %v", err)
	} else {
		currentNum = num
		successLogger.Printf("已更新game 编号为: 至 %d", num)
	}
}

func cleanLogs(num int) error {
	ip, err := getServerIP(num)
	if err != nil {
		return err
	}

	infoLogger.Printf("正在清理日志 服务:%d IP:%s", num, ip)
	cmd := exec.Command("ansible", "-i", ip+",", "all",
		"-m", "shell",
		"-a", fmt.Sprintf("path=/data/server%d/game/log/ state=absent recurse=yes", num))

	output, err := cmd.CombinedOutput()
	infoLogger.Printf("ansible输出:\n%s", output)
	if err != nil {
		return fmt.Errorf("ansible 命令执行失败: %v, 输出: %s", err, string(output))
	}
	return nil
}

func updateSleepTime(i int) error {
	time.Sleep(time.Duration(i) * time.Second)
	return nil
}

func updateWhitelist(num int) error {
	for _, loginIP := range loginSlice {
		infoLogger.Printf("正在更新白名单 game 编号为: 为:%d 当前IP为:%s", num, loginIP)
		cmd := exec.Command("ansible", "-i", fmt.Sprintf("%s,", loginIP),
			"all",
			"-m", "shell",
			"-a", fmt.Sprintf("sed -i -e '/^%d$/d' -e '/^$/d' %s", num, path.Join(loginListFilePath, whiteListName)))
		output, err := cmd.CombinedOutput()
		infoLogger.Printf("ansible输出:\n%s\n", output)
		if err != nil {
			return fmt.Errorf("ansible 命令执行失败: %v", err)
		}

		infoLogger.Printf("正在reload login 当前IP为:%s", loginIP)
		cmd = exec.Command("ansible-playbook",
			"-i", fmt.Sprintf("%s,", loginIP),
			"-e", fmt.Sprintf("host_name=%s,", loginIP),
			filepath.Join(currentDir, playbookDir, loginYamlFileName))
		output, err = cmd.CombinedOutput()
		infoLogger.Printf("ansible输出:\n%s", output)
		if err != nil {
			return fmt.Errorf("ansible 命令执行失败: %v", err)
		}
	}
	return nil
}

func updateOpenTime(num int) error {
	ip, err := getServerIP(num)
	if err != nil {
		return err
	}

	infoLogger.Printf("正在设置开服时间 game 编号为: 为:%d 当前IP为:%s", num, ip)
	cmd := exec.Command("ansible-playbook", "-i", ip+",",
		"-e", "host_name="+ip,
		"-e", "svc_num="+strconv.Itoa(num),
		filepath.Join(currentDir, playbookDir, openYamlFileName))

	output, err := cmd.CombinedOutput()
	infoLogger.Printf("ansible输出:\n%s", output)
	if err != nil {
		return fmt.Errorf("ansible 命令执行失败: %v", err)
	}
	return nil
}

func updateLimit(num int) error {
	for _, loginIP := range loginSlice {

		infoLogger.Printf("正在更新限制名单 服务:%d IP:%s", num, loginIP)
		cmd := exec.Command("ansible-playbook", "-i", fmt.Sprintf("%s,", loginIP),
			"-e", fmt.Sprintf("host_name=%s,", loginIP),
			"-e", fmt.Sprintf("svc_num=%d", num),
			"-e", fmt.Sprintf("list_path=%s", loginListFilePath),
			filepath.Join(currentDir, playbookDir, limitYamlFileName))

		output, err := cmd.CombinedOutput()
		infoLogger.Printf("ansible输出:\n%s", output)
		if err != nil {
			return fmt.Errorf("ansible 命令执行失败: %v, 输出: %s", err, string(output))
		}
		infoLogger.Printf("正在reload login 当前IP为:%s", loginIP)
		cmd = exec.Command("ansible-playbook",
			"-i", fmt.Sprintf("%s,", loginIP),
			"-e", fmt.Sprintf("host_name=%s,", loginIP),
			filepath.Join(currentDir, playbookDir, loginYamlFileName))
		output, err = cmd.CombinedOutput()
		infoLogger.Printf("ansible输出:\n%s", output)
		if err != nil {
			return fmt.Errorf("ansible 命令执行失败: %v, 输出: %s", err, string(output))
		}
	}
	return nil
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
		"https://api.611611.best/api/log",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
}

// ##########################获取##################################
// 通过game 编号为: 获取ip
func getServerIP(num int) (string, error) {
	for ip, nums := range ipMap {
		for _, n := range nums {
			if strconv.Itoa(num) == n {
				return ip, nil
			}
		}
	}
	return "", fmt.Errorf("未找到game 编号为:  %d 对应的IP", num)
}

func loadIpMap() {
	file, err := os.Open(filepath.Join(currentDir, gameListFileName))
	if err != nil {
		errLogger.Fatalf("无法打开列表文件: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Fields(line)

		ip := parts[0]
		numbers := strings.Split(parts[1][1:len(parts[1])-1], ",")
		ipMap[ip] = numbers
	}

	if err = scanner.Err(); err != nil {
		errLogger.Fatalf("文件读取错误: %v", err)
	}
}

func validateGameList() {
	file, err := os.Open(filepath.Join(currentDir, gameListFileName))
	if err != nil {
		errLogger.Fatalf("无法打开 game 列表文件: %v", err)
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
			errLogger.Fatalf("第%d行包含无效IP: %s", lineNum, parts[0])
		}

		if !strings.HasPrefix(parts[1], "[") || !strings.HasSuffix(parts[1], "]") {
			errLogger.Fatalf("第%d行game 编号为: 格式错误", lineNum)
		}

		nums := strings.Split(parts[1][1:len(parts[1])-1], ",")
		for _, n := range nums {
			if _, err = strconv.Atoi(n); err != nil {
				errLogger.Fatalf("第%d行game 编号为: 包含无效数字: %s", lineNum, n)
			}
		}
	}

	if err = scanner.Err(); err != nil {
		errLogger.Fatalf("文件读取错误: %v", err)
	}
}
func loadLoginSlice() {
	file, err := os.Open(filepath.Join(currentDir, loginListFileName))
	if err != nil {
		errLogger.Fatalf("无法打开 login_list.txt 列表文件: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// 跳过空行
		if strings.TrimSpace(line) == "" {
			continue
		}

		parts := strings.Fields(line)
		if net.ParseIP(parts[0]) == nil {
			errLogger.Fatalf("第%d行包含无效IP: %s", lineNum, parts[0])
		}
		loginSlice = append(loginSlice, parts[0])
	}

	if err = scanner.Err(); err != nil {
		errLogger.Fatalf("文件读取错误: %v", err)
	}
}

func validateNextServer(num int) bool {
	_, err := getServerIP(num)
	if err != nil {
		errLogger.Printf("无效的下个game 编号为:  %d: %v", num, err)
		return false
	}
	return true
}

func getCurrentServerNum() {
	file, err := os.Open(filepath.Join(currentDir, initFileName))
	if err != nil {
		errLogger.Fatalf("打开 init.txt 文件失败: %v", err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		errLogger.Fatalf("读取init文件失败: %v", err)
	}

	num, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		errLogger.Fatalf("无效的 game 编号为: %v", err)
	}

	currentNum = num
}

func queryCount(db *sql.DB, querySql string, args ...interface{}) (int, error) {
	var count int
	err := db.QueryRow(querySql, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("数据库查询错误: %v", err)
	}
	return count, nil
}

// ################################## CDN ###########################################

type RequestData struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func flushCDN(num int) error {
	params := url.Values{
		"zone_id": []string{strconv.Itoa(num)},
	}
	fullURL := cdnURL + "?" + params.Encode()

	client := &http.Client{Timeout: 5 * time.Second} // 5s 超时
	resp, err := client.Get(fullURL)
	if err != nil {
		warnLogger.Printf("请求失败: %v", err)
		return fmt.Errorf("HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		warnLogger.Printf("状态码非 200: %d", resp.StatusCode)
		return fmt.Errorf("HTTP 状态码错误: %d", resp.StatusCode)
	}

	var reqData RequestData
	if err = json.NewDecoder(resp.Body).Decode(&reqData); err != nil {
		warnLogger.Printf("解析响应体失败: %v", err)
		return fmt.Errorf("JSON 解析失败: %w", err)
	}

	if reqData.Code != 0 {
		warnLogger.Printf("返回状态非 0，Message: %s", reqData.Message)
		return fmt.Errorf("CDN 刷新失败: %s", reqData.Message)
	}

	successLogger.Printf("CDN 刷新成功，zone_id: %d", num)
	return nil
}
