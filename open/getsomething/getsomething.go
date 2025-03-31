package getsomething

import (
	"bufio"
	"database/sql"
	"fmt"
	"io"
	"net"
	"open/loglevel"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var errLogger = loglevel.GetErrLogger()
var warnLogger = loglevel.GetWarnLogger()
var successLogger = loglevel.GetSuccessLogger()

// GetCurrentDir 获取当前工作目录。
// 返回值:
//
//	string: 当前工作目录的路径。
//
// 如果获取失败，程序将通过 errLogger 记录错误并退出。
func GetCurrentDir() string {
	pwd, err := os.Getwd()
	if err != nil {
		errLogger.Fatalf("获取当前目录失败: %v", err)
	}
	return pwd
}

// GetGameIP 通过 game 编号获取对应的 IP 地址。
// 参数:
//
//	num: 要查询的 game 编号。
//	ipMap: 存储 IP 地址与 game 编号列表的映射表，键为 IP 地址，值为编号字符串切片。
//
// 返回值:
//
//	string: 找到的 IP 地址，如果未找到则返回空字符串。
//	error: 如果未找到对应的 IP，则返回错误信息；否则返回 nil。
func GetGameIP(num int, ipMap map[string][]string) (string, error) {
	for ip, nums := range ipMap {
		for _, n := range nums {
			if strconv.Itoa(num) == n {
				return ip, nil
			}
		}
	}
	return "", fmt.Errorf("未找到 game 编号为: %d 对应的IP", num)
}

// GetGroupID 通过 game 编号获取对应的 group ID。
// 参数:
//
//	num: 要查询的 game 编号。
//	ipGroup: 存储 IP 地址与 group ID 的映射表，键为 IP 地址，值为 group ID。
//	ipMap: 存储 IP 地址与 game 编号列表的映射表，键为 IP 地址，值为编号字符串切片。
//
// 返回值:
//
//	int: 找到的 group ID，如果未找到则返回 0。
//	error: 如果未找到对应的 IP 或 group ID，则返回错误信息；否则返回 nil。
func GetGroupID(num int, ipGroup map[string]int, ipMap map[string][]string) (int, error) {
	tmpIP, err := GetGameIP(num, ipMap)
	if err != nil {
		return 0, fmt.Errorf("未找到 game 编号为: %d 对应的IP", num)
	}
	for ip, groupID := range ipGroup {
		if ip == tmpIP {
			return groupID, nil
		}
	}
	return 0, fmt.Errorf("未找到 game 编号为: %d 对应的 group_id", num)
}

// GetCurrentGameNum 从指定文件中读取当前 game 编号。
// 参数:
//
//	currentDir: 文件所在目录。
//	initFileName: 包含 game 编号的文件名。
//
// 返回值:
//
//	int: 文件中记录的 game 编号。
//
// 如果文件操作或格式有误，程序将通过 errLogger 记录错误并退出。
func GetCurrentGameNum(currentDir, initFileName string) int {
	file, err := os.Open(filepath.Join(currentDir, initFileName))
	if err != nil {
		errLogger.Fatalf("打开 init.txt 文件失败: %v", err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		errLogger.Fatalf("读取init文件内容失败: %v", err)
	}

	num, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		errLogger.Fatalf("无效的 game 编号为: %v", err)
	}

	return num
}

// LoadIpMap 从 game_list.txt 文件加载 IP 和 game 编号的映射关系。
// 参数:
//
//	currentDir: 文件所在目录。
//	gameListFileName: 包含 IP 和 game 编号列表的文件名。
//
// 返回值:
//
//	ipMap: IP 地址到 game 编号列表的映射。
//	ipGroup: IP 地址到 group ID 的映射。
//
// 如果文件操作失败，程序将通过 errLogger 记录错误并退出。
func LoadIpMap(currentDir, gameListFileName string) (ipMap map[string][]string, ipGroup map[string]int) {
	file, err := os.Open(filepath.Join(currentDir, gameListFileName))
	if err != nil {
		errLogger.Fatalf("无法打开列表文件: %v", err)
	}
	defer file.Close()

	ipMap = make(map[string][]string) // 初始化 ipMap
	ipGroup = make(map[string]int)    // 初始化 ipGroup

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

		groupID, err := strconv.Atoi(parts[2])
		if err != nil {
			errLogger.Printf("解析 groupID 失败，IP: %s, 值: %s", ip, parts[2])
			continue
		}
		ipGroup[ip] = groupID
	}

	if err = scanner.Err(); err != nil {
		errLogger.Fatalf("文件读取错误: %v", err)
	}
	return ipMap, ipGroup
}

// ValidGameList 验证 game_list.txt 文件的格式和内容是否有效。
// 参数:
//
//	currentDir: 文件所在目录。
//	gameListFileName: 包含 IP 和 game 编号列表的文件名。
//
// 如果验证失败，程序将通过 errLogger 记录错误并退出。
func ValidGameList(currentDir, gameListFileName string) {
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
			errLogger.Fatalf("第%d行 game 编号格式错误", lineNum)
		}

		nums := strings.Split(parts[1][1:len(parts[1])-1], ",")
		for _, n := range nums {
			if _, err = strconv.Atoi(n); err != nil {
				errLogger.Fatalf("第%d行game 编号为: 包含无效数字: %s", lineNum, n)
			}
		}

		if _, err = strconv.Atoi(parts[2]); err != nil {
			errLogger.Fatalf("第%d行的 group_id 无效", lineNum)
		}
	}

	if err = scanner.Err(); err != nil {
		errLogger.Fatalf("文件读取错误: %v", err)
	}
}

// GetLoginSlice 从 login_list.txt 文件加载登录 IP 列表。
// 参数:
//
//	currentDir: 文件所在目录。
//	loginListFileName: 包含登录 IP 的文件名。
//
// 返回值:
//
//	loginSlice: 登录 IP 地址的切片。
//
// 如果文件操作或格式有误，程序将通过 errLogger 记录错误并退出。
func GetLoginSlice(currentDir, loginListFileName string) (loginSlice []string) {
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
		if len(parts) == 0 {
			errLogger.Fatalf("第%d行没有有效字段", lineNum)
		}
		if net.ParseIP(parts[0]) == nil {
			errLogger.Fatalf("第%d行包含无效IP: %s", lineNum, parts[0])
		}
		loginSlice = append(loginSlice, parts[0])
	}

	if err = scanner.Err(); err != nil {
		errLogger.Fatalf("文件读取错误: %v", err)
	}

	return loginSlice
}

// ValidNextServer 验证给定的 game 编号是否有效。
// 参数:
//
//	num: 要验证的 game 编号。
//	ipMap: 存储 IP 地址与 game 编号列表的映射表。
//
// 返回值:
//
//	bool: 如果编号有效返回 true，否则返回 false 并记录警告日志。
func ValidNextServer(num int, ipMap map[string][]string) bool {
	_, err := GetGameIP(num, ipMap)
	if err != nil {
		errLogger.Printf("无效的下个 game 编号为: %d: %v", num, err)
		return false
	}
	return true
}

// InitDB 初始化并返回 MySQL 数据库连接。
// 参数:
//
//	logDBUser: 数据库用户名。
//	logDBPassword: 数据库密码。
//	logDBHost: 数据库主机地址。
//	logDBPort: 数据库端口。
//	logDBName: 数据库名称。
//
// 返回值:
//
//	*sql.DB: 成功连接的数据库对象。
//	error: 如果连接失败，返回错误信息；否则返回 nil。
func InitDB(logDBUser, logDBPassword, logDBHost, logDBPort, logDBName string) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", logDBUser, logDBPassword, logDBHost, logDBPort, logDBName)
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
