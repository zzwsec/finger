package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"open/cdn"
	"open/execute"
	"open/getsomething"
	"open/loglevel"
)

const (
	loginListFileName   = "login_list.txt"
	gameListFileName    = "game_list.txt"
	initFileName        = "init.txt"
	limitYamlFileName   = "limit.yaml"
	loginYamlFileName   = "login.yaml"
	openYamlFileName    = "open.yaml"
	packageYamlFileName = "package.yaml"
	installYamlFileName = "install.yaml"
	playbookDir         = "playbook"
	whiteListName       = "white_list.txt"

	registerCountSql = "select count(1) register_num from log_register where zone_id=?;"
	rechargeCountSql = "select count(distinct player_id) as recharge_num from (select player_id, sum(money) as total from log_recharge where zone_id=? group by player_id having total>=?) as subquery;"
)

type InstallStruct struct {
	domain         string
	thread         int
	payNotifyUrl   string
	zk1IP          string
	zk1Port        int
	zk2IP          string
	zk2Port        int
	zk3IP          string
	zk3Port        int
	gameDBHost     string
	gameDBUser     string
	gameDBPassword string
	gameIndexNum   int
}

var (
	// 环境变量
	workMode              string
	cdnURL                string
	loginListFilePath     string
	logDBHost             string
	logDBPort             int
	logDBUser             string
	logDBPassword         string
	logDBName             string
	criticalRegisterCount int
	criticalRechargeCount int
	criticalMoney         int
	sleepInterval         int
	installExamples       = new(InstallStruct)

	// 全局变量
	ipMap         = make(map[string][]string)
	ipGroup       = make(map[string]int)
	loginSlice    = make([]string, 0)
	db            *sql.DB
	currentDir    string
	currentNum    int
	basePath      string
	whitePath     string
	loginBookPath string
	limitBookPath string
	openBookPath  string

	// 日志
	infoLogger    *log.Logger
	successLogger *log.Logger
	warnLogger    *log.Logger
	errLogger     *log.Logger
)

func init() {
	loglevel.Init()
	infoLogger = loglevel.GetInfoLogger()
	successLogger = loglevel.GetSuccessLogger()
	warnLogger = loglevel.GetWarnLogger()
	errLogger = loglevel.GetErrLogger()

	var err error
	if err = loadEnv(); err != nil {
		errLogger.Fatalf("环境变量解析失败: %v", err)
	}

	currentDir = getsomething.GetCurrentDir()
	getsomething.ValidGameList(currentDir, gameListFileName)
	ipMap, ipGroup = getsomething.LoadIpMap(currentDir, gameListFileName)
	loginSlice = getsomething.GetLoginSlice(currentDir, loginListFileName)
	currentNum = getsomething.GetCurrentGameNum(currentDir, initFileName)

	// 设置文件路径
	basePath = filepath.Join(currentDir, playbookDir)
	whitePath = filepath.Join(loginListFilePath, whiteListName)
	loginBookPath = filepath.Join(currentDir, playbookDir, loginYamlFileName)
	limitBookPath = filepath.Join(currentDir, playbookDir, limitYamlFileName)
	openBookPath = filepath.Join(currentDir, playbookDir, openYamlFileName)

	db, err = getsomething.InitDB(logDBUser, logDBPassword, logDBHost, strconv.Itoa(logDBPort), logDBName)
	if err != nil {
		errLogger.Fatalf("数据库初始化失败: %v", err)
	}
}

func main() {
	defer func() {
		if err := recover(); err != nil {
			stack := debug.Stack()
			errLogger.Printf("Panic occurred: %v\nStack: %s", err, stack)
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go handleSignals(sigCh)

	defer db.Close()
	mainLoop(db)
}

func mainLoop(db *sql.DB) {
	for {
		if err := db.Ping(); err != nil {
			warnLogger.Printf("数据库连接失效，尝试重连")
			db, err = getsomething.InitDB(logDBUser, logDBPassword, logDBHost, strconv.Itoa(logDBPort), logDBName)
			if err != nil {
				errLogger.Panicf("数据库重连失败: %v", err)
			}
			infoLogger.Printf("数据库重连成功")
		}
		initFilePath := filepath.Join(currentDir, initFileName)

		registerCount, err := execute.QueryCount(db, registerCountSql, currentNum)
		if err != nil {
			warnLogger.Printf("查询注册人数失败: %v", err)
			time.Sleep(time.Duration(30) * time.Second)
			continue
		}
		infoLogger.Printf("当前注册人数 %d / %d, game 编号: %d", registerCount, criticalRegisterCount, currentNum)

		nextNum := currentNum + 1
		if !getsomething.ValidNextServer(nextNum, ipMap) {
			infoLogger.Printf("待配置 game%d 不存在: \n", nextNum)
			os.Exit(0)
		}

		rechargeCount, err := execute.QueryCount(db, rechargeCountSql, currentNum, criticalMoney)
		if err != nil {
			warnLogger.Printf("查询付费人数失败: %v", err)
			time.Sleep(time.Minute)
			continue
		}
		infoLogger.Printf("当前付费人数 %d / %d, 付费临界值: %d, game 编号: %d", rechargeCount, criticalRechargeCount, criticalMoney, currentNum)

		// 达到临界值
		if registerCount >= criticalRegisterCount || rechargeCount >= criticalRechargeCount {
			if handleServerSwitch(currentNum, nextNum) {
				if err = execute.UpdateServerNum(nextNum, initFilePath); err != nil {
					errLogger.Printf("更新game本地编号文件失败: %v", err)
				} else {
					currentNum = nextNum
				}
			} else {
				errLogger.Printf("game 编号: %d 开服期间出现异常\n", nextNum)
			}
			continue
		}

		time.Sleep(30 * time.Second)
	}
}

// 包装函数
func cleanLogsWrapper(num int) error {
	return execute.CleanLogs(num, ipMap)
}

func updateOpenTimeWrapper(num int) error {
	return execute.UpdateOpenTime(num, ipMap, openBookPath)
}

func updateWhitelistWrapper(num int) error {
	return execute.UpdateWhitelist(num, loginSlice, whitePath, loginBookPath)
}

func updateLimitWrapper(num int) error {
	return execute.UpdateLimit(num, loginSlice, loginListFilePath, limitBookPath, loginBookPath)
}

func flushCDNWrapper(num int) error {
	return cdn.FlushCDN(num, cdnURL)
}

func handleServerSwitch(oldNum, newNum int) bool {
	if !getsomething.ValidNextServer(newNum, ipMap) {
		errLogger.Printf("待配置 game%d 为不存在: \n", newNum)
		return false
	}
	if workMode == "auto" {
		err := execute.InstallGame(currentNum, newNum,
			basePath, packageYamlFileName, installYamlFileName,
			ipMap, ipGroup)
		if err != nil {
			errLogger.Panicf(fmt.Sprintln(err))
		}
	}

	ops := []struct {
		name string
		fn   func(int) error
		arg  int
	}{
		{"清理日志", cleanLogsWrapper, newNum},
		{"开服时间", updateOpenTimeWrapper, newNum},
		{"白名单更新", updateWhitelistWrapper, newNum},
		{"CDN刷新", flushCDNWrapper, newNum},
		{"休眠间隔", execute.UpdateSleepTime, sleepInterval},
		{"限制名单", updateLimitWrapper, oldNum},
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
	const maxDelay = 10 * time.Second
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		infoLogger.Printf("执行 %s (尝试 %d/%d)", opName, attempt, maxRetries)

		if err := fn(arg); err != nil {
			lastErr = err
			warnLogger.Printf("%s 尝试 %d 失败: %v", opName, attempt, err)
			if attempt < maxRetries {
				delay := time.Duration(attempt*attempt) * time.Second
				if delay > maxDelay {
					delay = maxDelay
				}
				warnLogger.Printf("%s 等待 %v 后重试...", opName, delay)
				time.Sleep(delay)
			}
			continue
		}

		successLogger.Printf("任务 %s 成功", opName)
		return true
	}

	errLogger.Printf("任务 %s 失败 (共尝试 %d 次)，最后错误: %v", opName, maxRetries, lastErr)
	return false
}

func handleSignals(ch <-chan os.Signal) {
	sig := <-ch
	successLogger.Printf("收到退出信号: %v，手动退出", sig)
	os.Exit(0)
}

func loadEnv() error {
	var err error
	workMode = os.Getenv("workMode")
	cdnURL = os.Getenv("cdnURL")
	loginListFilePath = os.Getenv("loginListFilePath")
	logDBHost = os.Getenv("logDBHost")
	logDBUser = os.Getenv("logDBUser")
	logDBPassword = os.Getenv("logDBPassword")
	logDBName = os.Getenv("logDBName")
	installExamples.domain = os.Getenv("domain")
	installExamples.payNotifyUrl = os.Getenv("payNotifyUrl")
	installExamples.zk1IP = os.Getenv("zk1IP")
	installExamples.zk2IP = os.Getenv("zk2IP")
	installExamples.zk3IP = os.Getenv("zk3IP")
	installExamples.gameDBHost = os.Getenv("gameDBHost")
	installExamples.gameDBUser = os.Getenv("gameDBUser")
	installExamples.gameDBPassword = os.Getenv("gameDBPassword")

	logDBPort, err = strconv.Atoi(os.Getenv("logDBPort"))
	if err != nil || logDBPort <= 0 {
		return errors.New("日志数据库端口无效")
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
	installExamples.thread, err = strconv.Atoi(os.Getenv("thread"))
	if err != nil || installExamples.thread < 0 {
		return errors.New("线程数无效")
	}
	installExamples.zk1Port, err = strconv.Atoi(os.Getenv("zk1Port"))
	if err != nil || installExamples.zk1Port < 0 {
		return errors.New("zk1端口无效")
	}
	installExamples.zk2Port, err = strconv.Atoi(os.Getenv("zk2Port"))
	if err != nil || installExamples.zk2Port < 0 {
		return errors.New("zk2端口无效")
	}
	installExamples.zk3Port, err = strconv.Atoi(os.Getenv("zk3Port"))
	if err != nil || installExamples.zk3Port < 0 {
		return errors.New("zk3端口无效")
	}
	installExamples.gameIndexNum, err = strconv.Atoi(os.Getenv("gameIndexNum"))
	if err != nil || installExamples.gameIndexNum < 0 {
		return errors.New("gameIndexNum无效")
	}

	if cdnURL == "" || loginListFilePath == "" || logDBHost == "" || logDBUser == "" ||
		logDBPassword == "" || logDBName == "" || installExamples.domain == "" || installExamples.payNotifyUrl == "" ||
		installExamples.zk1IP == "" || installExamples.zk2IP == "" || installExamples.zk3IP == "" ||
		installExamples.gameDBHost == "" || installExamples.gameDBUser == "" || installExamples.gameDBPassword == "" {
		return errors.New("环境变量配置不齐全，请检查环境变量")
	}

	if workMode != "auto" && workMode != "manual" {
		return errors.New("工作模式只支持 auto 或 manual")
	}
	if net.ParseIP(logDBHost) == nil {
		return errors.New("日志数据库IP地址格式不正确")
	}
	if net.ParseIP(installExamples.gameDBHost) == nil {
		return errors.New("game数据库IP地址格式不正确")
	}

	return nil
}
