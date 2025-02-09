package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"gopkg.in/yaml.v3"
)

// 定义变量来存储从 YAML 文件中读取的值
var (
	pidFile    string
	initFile   string
	scriptPath string
	host       string
	port       string
	user       string
	password   string
	database   string
)

// 从 YAML 文件中加载配置
func loadConfig() error {
	data, err := os.ReadFile("args.yaml")
	if err != nil {
		return fmt.Errorf("无法读取 YAML 文件: %v", err)
	}

	// 定义一个 map 来存储 YAML 文件中的键值对
	config := make(map[string]string)

	// 解析 YAML 文件
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return fmt.Errorf("无法解析 YAML 文件: %v", err)
	}

	// 将 YAML 文件中的值赋给变量
	pidFile = config["pidFile"]
	initFile = config["initFile"]
	scriptPath = config["scriptPath"]
	host = config["host"]
	port = config["port"]
	user = config["user"]
	password = config["password"]
	database = config["database"]

	return nil
}

func main() {
	// 加载配置
	err := loadConfig()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 检查 PID 文件是否存在
	if checkPIDFile() {
		log.Println("程序已经在运行，退出")
		return
	}

	// 将当前进程的 PID 写入文件
	err = writePIDFile()
	if err != nil {
		log.Fatalf("无法写入 PID 文件: %v", err)
	}
	defer removePIDFile() // 程序退出时删除 PID 文件

	// 连接数据库
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", user, password, host, port, database)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer db.Close()

	// 保持数据库连接，优化连接池
	db.SetMaxOpenConns(2)                   // 增加最大连接数，防止过多连接导致堵塞
	db.SetMaxIdleConns(2)                   // 设置空闲连接数，以减少频繁连接关闭带来的性能损耗
	db.SetConnMaxLifetime(30 * time.Minute) // 设置连接最大存活时间，避免过期连接

	for {
		// 从文件读取当前服务编号
		data, err := os.ReadFile(initFile)
		if err != nil {
			log.Printf("读取文件失败: %v", err)
			time.Sleep(30 * time.Second)
			continue
		}
		// 去除前后的空白字符（包括换行符）
		strData := strings.TrimSpace(string(data))

		// 将文件内容转换为整数
		runSvcNum, err := strconv.Atoi(strData) // 转换为 int 类型
		if err != nil {
			log.Printf("转换服务编号失败: %v", err)
			time.Sleep(30 * time.Second)
			continue
		}

		// 参数化查询
		querySql := "SELECT COUNT(id) FROM log_register WHERE zone_id=?"
		// 定义变量存储查询结果
		var tmpRunSvcCount, tmpRunSvcNextCount int

		// 执行查询并将查询结果放到变量tmpRunSvcCount
		err = db.QueryRow(querySql, runSvcNum).Scan(&tmpRunSvcCount)
		if err != nil {
			log.Printf("查询当前服务玩家数失败: %v", err)
			time.Sleep(30 * time.Second)
			continue
		}

		err = db.QueryRow(querySql, runSvcNum+1).Scan(&tmpRunSvcNextCount)
		if err != nil {
			log.Printf("查询下一个服务玩家数失败: %v", err)
			time.Sleep(30 * time.Second)
			continue
		}

		// 确保查询返回的结果非空且符合条件
		if tmpRunSvcCount >= 1000 && tmpRunSvcNextCount == 0 {
			// 如果符合条件，执行安装脚本
			err := executeInstallScript(strconv.Itoa(runSvcNum + 1))
			if err != nil {
				log.Printf("执行安装脚本失败: %v", err)
			} else {
				// 更新服务编号
				err := os.WriteFile(initFile, []byte(fmt.Sprintf("%d", runSvcNum+1)), 0644)
				if err != nil {
					log.Printf("更新文件失败: %v", err)
				}
			}
		}

		// 等待30秒后再次执行
		time.Sleep(30 * time.Second)
	}
}

// 执行安装脚本（增加指数退避机制）
func executeInstallScript(runSvcNum string) error {
	const (
		maxRetries   = 3
		initialDelay = 60 * time.Second // 初始等待时间
	)

	// 打开日志文件（追加模式）
	logFile, err := os.OpenFile("info.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("无法打开日志文件: %v\n", err)
		return err
	}
	defer logFile.Close()

	// 创建日志记录器
	logger := log.New(logFile, "", log.LstdFlags)

	delay := initialDelay // 当前等待时间

	for attempt := 1; attempt <= maxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		cmd := exec.CommandContext(ctx, scriptPath, runSvcNum, "1")
		cmd.Stdout = nil
		cmd.Stderr = nil

		startTime := time.Now()
		err := cmd.Run()
		cancel() // 确保每次循环结束后释放资源

		if err == nil {
			successMsg := fmt.Sprintf("脚本执行成功, 服务器编号: %s (耗时: %v)",
				runSvcNum, time.Since(startTime))
			logger.Println(successMsg)
			return nil
		}

		// 错误处理
		errorMsg := fmt.Sprintf("执行失败 (第 %d 次尝试): %v (耗时: %v)",
			attempt, err, time.Since(startTime))
		logger.Println(errorMsg)

		if attempt == maxRetries {
			logger.Println("连续 3 次执行失败，退出程序")
			os.Exit(1)
		}

		// 指数退避等待
		logger.Printf("等待 %v 后重试...", delay)
		time.Sleep(delay)
		delay *= 2 // 等待时间翻倍
	}

	return fmt.Errorf("脚本执行失败")
}

// 检查 PID 文件，判断是否已有正在运行的程序
func checkPIDFile() bool {
	if _, err := os.Stat(pidFile); os.IsNotExist(err) {
		return false
	}

	data, err := os.ReadFile(pidFile)
	if err != nil {
		log.Printf("无法读取 PID 文件: %v，删除文件", err)
		os.Remove(pidFile)
		return false
	}

	storedPID, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		log.Printf("PID 文件内容无效: %v，删除文件", err)
		os.Remove(pidFile)
		return false
	}

	process, err := os.FindProcess(storedPID)
	if err != nil {
		log.Printf("无法查找进程 PID %d: %v，删除文件", storedPID, err)
		os.Remove(pidFile)
		return false
	}

	if err := process.Signal(syscall.Signal(0)); err == nil {
		log.Printf("程序已在运行 (PID: %d)", storedPID)
		return true
	} else {
		log.Printf("进程 %d 不存在，删除 PID 文件", storedPID)
		os.Remove(pidFile)
		return false
	}
}

// 写入当前进程的 PID 到文件
func writePIDFile() error {
	// 获取当前进程的 PID
	pid := os.Getpid()
	pidStr := strconv.Itoa(pid)

	// 写入 PID 到文件
	return os.WriteFile(pidFile, []byte(pidStr), 0644)
}

// 删除 PID 文件
func removePIDFile() {
	err := os.Remove(pidFile)
	if err != nil {
		log.Printf("无法删除 PID 文件: %v", err)
	}
}
