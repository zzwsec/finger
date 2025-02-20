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
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	listFile         = "/open/list.txt"
	initFile         = "/open/init.txt"
	whiteYamlFile    = "/open/playbook/white.yaml"
	openYamlFile     = "/open/playbook/open.yaml"
	limitYamlFile    = "/open/playbook/limit.yaml"
	argsYaml         = "/open/args.yaml"
	registerCountSql = "select count(1) register_num from log_register where zone_id=?"
	rechargeCountSql = "select count(distinct player_id) as recharge_num from (select player_id, sum(money) as total from log_recharge where zone_id=? group by player_id having total>=6) as subquery"
)

var (
	dbHost     string
	dbPort     string
	dbUser     string
	dbPassword string
	dbName     string

	ipMap  = make(map[string][]string)
	logger *log.Logger
)

func main() {
	defer func() {
		if err := recover(); err != nil {
			SendMessage(fmt.Sprintf("Panic occurred: %v", err))
			os.Exit(1)
		}
	}()

	// 初始化日志器
	logger = log.New(os.Stdout, "MESSAGE ", log.Ldate|log.Ltime)

	// 前置验证
	validateListFile()
	loadIpMap()

	// 加载配置
	if err := loadConfig(); err != nil {
		logger.Panicf("[ERROR]: args.yaml文件加载或解析失败: %v", err)
	}

	// 信号处理
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go handleSignals(sigCh)

	// 数据库连接
	db, err := initDB()
	if err != nil {
		logger.Panicf("[ERROR]: 数据库初始化失败: %v", err)
	}
	defer db.Close()

	mainLoop(db)
}

func mainLoop(db *sql.DB) {
	for {
		currentNum := getCurrentServerNum()
		nextNum := currentNum + 1

		if err := db.Ping(); err != nil {
			logger.Panicf("[ERROR]: 数据库连接关闭")
		}

		registerCount, err := queryCount(db, currentNum, registerCountSql)
		if err != nil {
			logger.Panicf("[ERROR]: 当前服务 %d 查询注册人数失败: %v", currentNum, err)
		}
		logger.Printf("[INFO]: 当前检查注册人数 %d，服务编号为: %d", registerCount, currentNum)

		if registerCount >= 2000 {
			if handleServerSwitch(currentNum, nextNum) {
				updateServerNum(nextNum)
				continue
			} else {
				panic(fmt.Sprintf("[ERROR]: 服务编号%d, 开服失败!", nextNum))
			}
		}

		rechargeCount, err := queryCount(db, currentNum, rechargeCountSql)
		if err != nil {
			logger.Panicf("[ERROR]: 当前服务 %d 付费人数查询失败: %v", currentNum, err)
		}
		logger.Printf("[INFO]: 当前检查付费人数 %d，服务编号为: %d", rechargeCount, currentNum)

		if rechargeCount >= 100 {
			if handleServerSwitch(currentNum, nextNum) {
				updateServerNum(nextNum)
				continue
			} else {
				panic(fmt.Sprintf("[ERROR]: 服务编号%d, 开服失败!", nextNum))
			}
		}

		sleep()
	}
}

func loadConfig() error {
	data, err := os.ReadFile(argsYaml)
	if err != nil {
		return fmt.Errorf("[ERROR]: 读取配置失败: %v", err)
	}

	config := make(map[string]string)
	if err = yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("[ERROR]: 解析配置失败: %v", err)
	}

	// 设置配置项
	dbHost = config["dbHost"]
	dbPort = config["dbPort"]
	dbUser = config["dbUser"]
	dbPassword = config["dbPassword"]
	dbName = config["dbName"]

	return nil
}

func handleSignals(ch <-chan os.Signal) {
	sig := <-ch
	logger.Printf("[INFO]: 收到信号: %v，执行清理", sig)
	SendMessage("[SUCCESS]: 用户手动退出")
	os.Exit(0)
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
			log.Printf("[WARNING]: 尝试 %d: 创建数据库对象失败: %v", i+1, err)
			continue // 直接重试
		}

		// 验证连接有效性
		err = db.Ping()
		if err == nil {
			break // 连接成功，退出循环
		}

		// 关闭无效连接并等待重试
		log.Printf("[WARNING]: 尝试 %d 次: 数据库连接失败: %v", i+1, err)
		db.Close()
		time.Sleep(retryDelay)
		retryDelay *= 2 // 指数退避，避免雪崩
	}

	if err != nil {
		return nil, fmt.Errorf("[ERROR]: 数据库连接失败（共尝试 %d 次）: %v", maxRetries, err)
	}

	// 4. 配置连接池
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	logger.Println("[INFO]: 数据库连接成功")
	return db, nil
}

func getCurrentServerNum() int {
	file, err := os.Open(initFile)
	if err != nil {
		logger.Panicf("[ERROR]: 打开init文件失败: %v", err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		logger.Panicf("[ERROR]: 读取init文件失败: %v", err)
	}

	num, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		logger.Panicf("[ERROR]: 无效的服务编号: %v", err)
	}

	return num
}

func queryCount(db *sql.DB, zoneID int, querySql string) (int, error) {
	var count int
	err := db.QueryRow(querySql, zoneID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("[ERROR]: 数据库查询错误: %v", err)
	}
	return count, nil
}

func sleep() {
	time.Sleep(30 * time.Second)
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
		logger.Printf("[INFO]: 执行 %s (尝试 %d/%d)", opName, attempt, maxRetries)

		if err := fn(arg); err != nil {
			lastErr = err
			logger.Printf("[INFO]: %s 尝试 %d 失败: %v", opName, attempt, err)

			// 指数退避等待
			if sleepDuration := time.Duration(attempt*attempt) * time.Second; attempt < maxRetries {
				logger.Printf("[INFO]: %s 等待 %v 后重试...", opName, sleepDuration)
				time.Sleep(sleepDuration)
			}
			continue
		}

		logger.Printf("[SUCCESS]: %s 成功", opName)
		return true
	}

	logger.Printf("[INFO]: %s 失败 (共尝试 %d 次)，最后错误: %v", opName, maxRetries, lastErr)
	return false
}

func validateNextServer(num int) bool {
	_, err := getServerIP(num)
	if err != nil {
		logger.Printf("[INFO]: 无效的下个服务编号 %d: %v", num, err)
		return false
	}
	return true
}

func getServerIP(num int) (string, error) {
	for ip, nums := range ipMap {
		for _, n := range nums {
			if strconv.Itoa(num) == n {
				return ip, nil
			}
		}
	}
	return "", fmt.Errorf("[ERROR]: 未找到服务编号 %d 对应的IP", num)
}

func updateWhitelist(num int) error {
	ip, err := getServerIP(num)
	if err != nil {
		return err
	}

	logger.Printf("[INFO]: 正在更新白名单 服务:%d IP:%s", num, ip)
	cmd := exec.Command("ansible-playbook", "-i", ip+",",
		"-e", "host_name="+ip,
		"-e", "white_num="+strconv.Itoa(num),
		whiteYamlFile)

	output, err := cmd.CombinedOutput()
	logger.Printf("[INFO]: Ansible输出:\n%s", output)

	return err
}

func updateOpenTime(num int) error {
	ip, err := getServerIP(num)
	if err != nil {
		return err
	}

	logger.Printf("[INFO]: 正在设置开放时间 服务:%d IP:%s", num, ip)
	cmd := exec.Command("ansible-playbook", "-i", ip+",",
		"-e", "host_name="+ip,
		"-e", "svc_num="+strconv.Itoa(num),
		openYamlFile)

	output, err := cmd.CombinedOutput()
	logger.Printf("[INFO]: Ansible输出:\n%s", output)

	return err
}

func updateLimit(num int) error {
	ip, err := getServerIP(num)
	if err != nil {
		return err
	}

	logger.Printf("[INFO]: 正在更新限制名单 服务:%d IP:%s", num, ip)
	cmd := exec.Command("ansible-playbook", "-i", ip+",",
		"-e", "host_name="+ip,
		"-e", "svc_num="+strconv.Itoa(num),
		limitYamlFile)

	output, err := cmd.CombinedOutput()
	logger.Printf("[INFO]: Ansible输出:\n%s", output)

	return err
}

func cleanLogs(num int) error {
	ip, err := getServerIP(num)
	if err != nil {
		return err
	}

	logger.Printf("[INFO]: 正在清理日志 服务:%d IP:%s", num, ip)
	cmd := exec.Command("ansible", "-i", ip+",", "all",
		"-m", "shell",
		"-a", fmt.Sprintf("rm -rfv /data/server%d/game/log/*", num))

	output, err := cmd.CombinedOutput()
	logger.Printf("[INFO]: Ansible输出:\n%s", output)

	return err
}

func updateServerNum(num int) {
	if err := os.WriteFile(initFile, []byte(strconv.Itoa(num)+"\n"), 0644); err != nil {
		logger.Panicf("[ERROR]: 更新服务编号失败: %v", err)
	} else {
		logger.Printf("[INFO]: 已更新服务编号至 %d", num)
	}
}

func validateListFile() {
	file, err := os.Open(listFile)
	if err != nil {
		logger.Panicf("[ERROR]: 无法打开列表文件: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) != 2 {
			logger.Panicf("[ERROR]: 第%d行格式错误: 需要两个字段", lineNum)
		}

		if net.ParseIP(parts[0]) == nil {
			logger.Panicf("[ERROR]: 第%d行包含无效IP: %s", lineNum, parts[0])
		}

		if !strings.HasPrefix(parts[1], "[") || !strings.HasSuffix(parts[1], "]") {
			logger.Panicf("[ERROR]: 第%d行编号格式错误", lineNum)
		}

		nums := strings.Split(parts[1][1:len(parts[1])-1], ",")
		for _, n := range nums {
			if _, err := strconv.Atoi(n); err != nil {
				logger.Panicf("[ERROR]: 第%d行包含无效数字: %s", lineNum, n)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		logger.Panicf("[ERROR]: 文件读取错误: %v", err)
	}
}

func loadIpMap() {
	file, err := os.Open(listFile)
	if err != nil {
		logger.Panicf("[ERROR]: 无法打开列表文件: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}

		ip := parts[0]
		numbers := strings.Split(parts[1][1:len(parts[1])-1], ",")
		ipMap[ip] = numbers
	}

	if err := scanner.Err(); err != nil {
		logger.Panicf("[ERROR]: 文件读取错误: %v", err)
	}
}

type LogRequest struct {
	Action    string `json:"action"`
	Timestamp string `json:"timestamp"`
	Country   string `json:"country"`
	OSInfo    string `json:"os_info"`
	CPUArch   string `json:"cpu_arch"`
}

func SendMessage(action string) {
	country := getCountry()

	osInfo := getOSInfo()

	cpuArch := runtime.GOARCH
	if cpuArch == "" {
		cpuArch = "unknown"
	}

	// 发送请求
	sendLogRequest(LogRequest{
		Action:    action,
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		Country:   country,
		OSInfo:    osInfo,
		CPUArch:   cpuArch,
	})
}

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

// 发送请求
func sendLogRequest(data LogRequest) {
	jsonData, _ := json.Marshal(data)
	http.Post(
		"https://api.honeok.com/api/log",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
}