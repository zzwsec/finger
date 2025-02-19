package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"gopkg.in/yaml.v2"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	listFile      = "/data/ansible/open/list.txt"
	initFile      = "/data/ansible/open/init.txt"
	whiteYamlFile = "/data/ansible/open/playbook/white.yaml"
	openYamlFile  = "/data/ansible/open/playbook/open.yaml"
	limitYamlFile = "/data/ansible/open/playbook/limit.yaml"
	argsYaml      = "/data/ansible/open/args.yaml"
	querySql      = "SELECT COUNT(*) FROM log_register WHERE zone_id=?"
	defaultLog    = "/data/ansible/open/info.log"
)

var (
	dbHost     string
	dbPort     string
	dbUser     string
	dbPassword string
	dbName     string
	pidFile    string
	logPath    string

	ipMap   = make(map[string][]string)
	logger  *log.Logger
	logFile *os.File
)

func main() {
	// 初始化日志系统
	if err := initLogging(); err != nil {
		log.Fatalf("初始化日志失败: %v", err)
	}
	defer logFile.Close()

	// 前置验证
	validateListFile()
	loadIpMap()

	// 加载配置
	if err := loadConfig(); err != nil {
		logger.Fatalf("配置加载失败: %v", err)
	}

	// 单实例检查
	if checkRunning() {
		logger.Println("程序已在运行，退出")
		return
	}

	// 写入 PID 文件
	if err := writePID(); err != nil {
		logger.Printf("写入 PID 文件失败: %v", err)
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
		logger.Fatalf("数据库初始化失败: %v", err)
	}
	defer db.Close()

	mainLoop(db)
}

func initLogging() error {
	// 先加载基础配置获取日志路径
	if err := loadBasicConfig(); err != nil {
		return fmt.Errorf("基础配置加载失败: %v", err)
	}

	// 设置默认路径
	if logPath == "" {
		logPath = defaultLog
	}

	// 打开日志文件
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("无法打开日志文件: %v", err)
	}
	logFile = f

	// 配置日志器
	logger = log.New(logFile, "system-message ", log.Ldate|log.Ltime|log.Lshortfile)

	// 重定向标准输出
	os.Stdout = logFile
	os.Stderr = logFile

	return nil
}

func loadBasicConfig() error {
	data, err := os.ReadFile(argsYaml)
	if err != nil {
		return fmt.Errorf("读取YAML失败: %v", err)
	}

	config := make(map[string]string)
	if err = yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析YAML失败: %v", err)
	}

	// 获取日志路径
	logPath = config["logFile"]
	return nil
}

func loadConfig() error {
	data, err := os.ReadFile(argsYaml)
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
	pidFile = config["pidFile"]

	return nil
}

func checkRunning() bool {
	if _, err := os.Stat(pidFile); os.IsNotExist(err) {
		return false
	}

	data, err := os.ReadFile(pidFile)
	if err != nil {
		logger.Printf("读取PID文件失败: %v，尝试删除", err)
		os.Remove(pidFile)
		return false
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		logger.Printf("无效的PID内容: %v，删除文件", err)
		os.Remove(pidFile)
		return false
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		logger.Printf("查找进程失败: %v，删除文件", err)
		os.Remove(pidFile)
		return false
	}

	if err := process.Signal(syscall.Signal(0)); err == nil {
		logger.Printf("进程 %d 正在运行", pid)
		return true
	}

	logger.Printf("进程 %d 不存在，删除PID文件", pid)
	os.Remove(pidFile)
	return false
}

func writePID() error {
	pid := os.Getpid()
	return os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0644)
}

func cleanup() {
	if _, err := os.Stat(pidFile); os.IsNotExist(err) {
		logger.Printf("PID 文件不存在，跳过删除")
		return
	}
	if err := os.Remove(pidFile); err != nil {
		logger.Printf("删除PID文件失败: %v", err)
	} else {
		logger.Println("PID文件删除成功")
	}
}

func handleSignals(ch <-chan os.Signal) {
	sig := <-ch
	logger.Printf("收到信号: %v，执行清理", sig)
	cleanup()
	os.Exit(0)
}

func initDB() (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s",
		dbUser, dbPassword, dbHost, dbPort, dbName)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("数据库连接失败: %v", err)
	}

	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("数据库连通性检查失败: %v", err)
	}

	logger.Println("数据库连接成功")
	return db, nil
}

func mainLoop(db *sql.DB) {
	for {
		currentNum := getCurrentServerNum()
		nextNum := currentNum + 1

		currentCount, err := queryCount(db, currentNum)
		if err != nil {
			logger.Printf("当前服务 %d 查询失败: %v", currentNum, err)
			sleep()
			continue
		}
		logger.Printf("当前检查服务编号为: %d", currentNum)

		nextCount, err := queryCount(db, nextNum)
		if err != nil {
			logger.Printf("下个服务 %d 查询失败: %v", nextNum, err)
			sleep()
			continue
		}

		if currentCount >= 1000 && nextCount == 0 {
			if handleServerSwitch(currentNum, nextNum) {
				updateServerNum(nextNum)
			}
		}

		sleep()
	}
}

func getCurrentServerNum() int {
	file, err := os.Open(initFile)
	if err != nil {
		logger.Fatalf("打开init文件失败: %v", err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		logger.Fatalf("读取init文件失败: %v", err)
	}

	num, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		logger.Fatalf("无效的服务编号: %v", err)
	}

	return num
}

func queryCount(db *sql.DB, zoneID int) (int, error) {
	var count int
	err := db.QueryRow(querySql, zoneID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("数据库查询错误: %v", err)
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
		{"清理日志", cleanLogs, oldNum},
	}

	for _, op := range ops {
		if err := op.fn(op.arg); err != nil {
			logger.Printf("%s失败: %v", op.name, err)
			return false
		}
		logger.Printf("%s成功", op.name)
	}

	return true
}

func validateNextServer(num int) bool {
	_, err := getServerIP(num)
	if err != nil {
		logger.Printf("无效的下个服务编号 %d: %v", num, err)
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
	return "", fmt.Errorf("未找到服务编号 %d 对应的IP", num)
}

func updateWhitelist(num int) error {
	ip, err := getServerIP(num)
	if err != nil {
		return err
	}

	logger.Printf("正在更新白名单 服务:%d IP:%s", num, ip)
	cmd := exec.Command("ansible-playbook", "-i", ip+",",
		"-e", "host_name="+ip,
		"-e", "white_num="+strconv.Itoa(num),
		whiteYamlFile)

	output, err := cmd.CombinedOutput()
	logger.Printf("Ansible输出:\n%s", output)

	return err
}

func updateOpenTime(num int) error {
	ip, err := getServerIP(num)
	if err != nil {
		return err
	}

	logger.Printf("正在设置开放时间 服务:%d IP:%s", num, ip)
	cmd := exec.Command("ansible-playbook", "-i", ip+",",
		"-e", "host_name="+ip,
		"-e", "svc_num="+strconv.Itoa(num),
		openYamlFile)

	output, err := cmd.CombinedOutput()
	logger.Printf("Ansible输出:\n%s", output)

	return err
}

func updateLimit(num int) error {
	ip, err := getServerIP(num)
	if err != nil {
		return err
	}

	logger.Printf("正在更新限制名单 服务:%d IP:%s", num, ip)
	cmd := exec.Command("ansible-playbook", "-i", ip+",",
		"-e", "host_name="+ip,
		"-e", "svc_num="+strconv.Itoa(num),
		limitYamlFile)

	output, err := cmd.CombinedOutput()
	logger.Printf("Ansible输出:\n%s", output)

	return err
}

func cleanLogs(num int) error {
	ip, err := getServerIP(num)
	if err != nil {
		return err
	}

	logger.Printf("正在清理日志 服务:%d IP:%s", num, ip)
	cmd := exec.Command("ansible", "-i", ip+",", "all",
		"-m", "shell",
		"-a", fmt.Sprintf("rm -rfv /data/server%d/game/log/*", num))

	output, err := cmd.CombinedOutput()
	logger.Printf("Ansible输出:\n%s", output)

	return err
}

func updateServerNum(num int) {
	if err := os.WriteFile(initFile, []byte(strconv.Itoa(num)+"\n"), 0644); err != nil {
		logger.Printf("更新服务编号失败: %v", err)
	} else {
		logger.Printf("已更新服务编号至 %d", num)
	}
}

func validateListFile() {
	file, err := os.Open(listFile)
	if err != nil {
		logger.Fatalf("无法打开列表文件: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) != 2 {
			logger.Fatalf("第%d行格式错误: 需要两个字段", lineNum)
		}

		if net.ParseIP(parts[0]) == nil {
			logger.Fatalf("第%d行包含无效IP: %s", lineNum, parts[0])
		}

		if !strings.HasPrefix(parts[1], "[") || !strings.HasSuffix(parts[1], "]") {
			logger.Fatalf("第%d行编号格式错误", lineNum)
		}

		nums := strings.Split(parts[1][1:len(parts[1])-1], ",")
		for _, n := range nums {
			if _, err := strconv.Atoi(n); err != nil {
				logger.Fatalf("第%d行包含无效数字: %s", lineNum, n)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		logger.Fatalf("文件读取错误: %v", err)
	}
}

func loadIpMap() {
	file, err := os.Open(listFile)
	if err != nil {
		logger.Fatalf("无法打开列表文件: %v", err)
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
		logger.Fatalf("文件读取错误: %v", err)
	}
}
