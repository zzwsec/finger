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
)

const (
	pidFile    = "./open.pid"
	initFile   = "/data/ansible/open/init.txt"
	scriptPath = "/data/ansible/open/install.sh"
	host       = "192.168.121.120"
	port       = "3306"
	user       = "root"
	password   = "root"
	database   = "cbt4_log"
)

func main() {
	// 检查 PID 文件是否存在
	if checkPIDFile() {
		log.Println("程序已经在运行，退出")
		return
	}

	// 将当前进程的 PID 写入文件
	err := writePIDFile()
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
		currentSql := "SELECT COUNT(id) FROM log_register WHERE zone_id=?"
		nextSql := "SELECT COUNT(id) FROM log_register WHERE zone_id=?"

		// 定义变量存储查询结果
		var tmpRunSvcCount, tmpRunSvcNextCount int

		// 执行查询并将查询结果放到变量tmpRunSvcCount
		err = db.QueryRow(currentSql, runSvcNum).Scan(&tmpRunSvcCount)
		if err != nil {
			log.Printf("查询当前服务玩家数失败: %v", err)
			time.Sleep(30 * time.Second)
			continue
		}

		err = db.QueryRow(nextSql, runSvcNum+1).Scan(&tmpRunSvcNextCount)
		if err != nil {
			log.Printf("查询下一个服务玩家数失败: %v", err)
			time.Sleep(30 * time.Second)
			continue
		}

		// 确保查询返回的结果非空且符合条件
		if tmpRunSvcCount > 1000 && tmpRunSvcNextCount == 0 {
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

// 执行安装脚本
func executeInstallScript(runSvcNum string) error {
	const maxRetries = 3 // 最大重试次数

	// 打开日志文件（追加模式）
	logFile, err := os.OpenFile("info.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("无法打开日志文件: %v\n", err)
		return err
	}
	defer logFile.Close()

	// 创建日志记录器
	logger := log.New(logFile, "", log.LstdFlags)

	for attempt := 1; attempt <= maxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		// 执行脚本并隐藏其输出
		cmd := exec.CommandContext(ctx, scriptPath, runSvcNum, "1")

		// **重定向输出，不显示在终端**
		cmd.Stdout = nil
		cmd.Stderr = nil

		// 运行命令
		err := cmd.Run()
		if err == nil {
			successMsg := fmt.Sprintf("脚本执行成功, 服务器编号: %s\n", runSvcNum)
			logger.Println(successMsg) // 记录成功日志
			return nil
		}

		// 失败时记录日志
		errorMsg := fmt.Sprintf("执行脚本失败 (第 %d 次尝试): %v\n", attempt, err)
		logger.Println(errorMsg) // 记录失败日志

		// 如果达到最大重试次数，则终止程序
		if attempt == maxRetries {
			finalMsg := "连续 3 次执行失败，退出程序\n"
			logger.Println(finalMsg)
			os.Exit(1) // 直接退出 Go 进程
		}

		logger.Println("等待 60 秒后重试...")
		time.Sleep(60 * time.Second) // 等待 60 秒后重试
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
