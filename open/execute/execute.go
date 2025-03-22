package execute

import (
	"database/sql"
	"fmt"
	"github.com/a8m/envsubst"
	"open/getsomething"
	"open/loglevel"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

const basePort = 12000

var (
	infoLogger    = loglevel.GetInfoLogger()
	successLogger = loglevel.GetSuccessLogger()
)

// UpdateServerNum 更新 init.txt 文件中的 game 编号。
// 参数:
//
//	num: 要更新的 game 编号。
//	initFilePath: init.txt 文件的路径。
//
// 返回值:
//
//	error: 如果文件写入失败，返回错误信息；否则返回 nil。
func UpdateServerNum(num int, initFilePath string) error {
	if err := os.WriteFile(initFilePath, []byte(strconv.Itoa(num)+"\n"), 0644); err != nil {
		return fmt.Errorf("更新 init.txt 文件中game 编号为: 失败: %v", err)
	}
	successLogger.Printf("已更新 game 编号至 %d", num)
	return nil
}

// CleanLogs 清理指定 game 编号的日志。
// 参数:
//
//	num: 要清理日志的 game 编号。
//	ipMap: IP 地址到 game 编号列表的映射表。
//
// 返回值:
//
//	error: 如果获取 IP 或清理日志失败，返回错误信息；否则返回 nil。
func CleanLogs(num int, ipMap map[string][]string) error {
	ip, err := getsomething.GetGameIP(num, ipMap)
	if err != nil {
		return err
	}

	infoLogger.Printf("正在清理日志, game 编号为:%d IP:%s", num, ip)
	cmd := exec.Command("ansible", "-i", fmt.Sprintf("%s,", ip),
		"all",
		"-m", "shell",
		"-a", fmt.Sprintf("path=/data/server%d/game/log/ state=absent recurse=yes", num))

	output, err := cmd.CombinedOutput()
	infoLogger.Printf("ansible输出:\n%s\n", output)
	if err != nil {
		return fmt.Errorf("ansible 清理日志失败: %v\n", err)
	}
	return nil
}

// UpdateSleepTime 暂停执行指定的秒数。
// 参数:
//
//	i: 暂停的秒数。
//
// 返回值:
//
//	error: 始终返回 nil。
func UpdateSleepTime(i int) error {
	time.Sleep(time.Duration(i) * time.Second)
	return nil
}

// UpdateWhitelist 更新指定 game 编号的白名单并重载登录服务。
// 参数:
//
//	num: 要更新的 game 编号。
//	loginSlice: 登录服务器 IP 列表。
//	whitePath: 白名单文件在被控节点路径。
//	loginBookPath: Ansible playbook 文件路径，用于重载登录服务。
//
// 返回值:
//
//	error: 如果更新白名单或重载登录服务失败，返回错误信息；否则返回 nil。
func UpdateWhitelist(num int, loginSlice []string, whitePath, loginBookPath string) error {
	fmt.Printf("白名单路径为: %s\n", whitePath)
	for _, loginIP := range loginSlice {
		infoLogger.Printf("正在更新白名单, game 编号为:%d 当前IP为:%s", num, loginIP)
		cmd := exec.Command("ansible", "-i", fmt.Sprintf("%s,", loginIP),
			"all",
			"-m", "shell",
			"-a", fmt.Sprintf("sed -i -e '/^%d$/d' -e '/^$/d' %s", num, whitePath))
		output, err := cmd.CombinedOutput()
		infoLogger.Printf("ansible输出:\n%s\n", output)
		if err != nil {
			return fmt.Errorf("ansible 更新白名单失败: %v\n", err)
		}

		infoLogger.Printf("正在 reload login 当前IP为:%s", loginIP)
		cmd = exec.Command("ansible-playbook",
			"-i", fmt.Sprintf("%s,", loginIP),
			"-e", fmt.Sprintf("host_name=%s,", loginIP),
			loginBookPath)
		output, err = cmd.CombinedOutput()
		infoLogger.Printf("ansible输出:\n%s\n", output)
		if err != nil {
			return fmt.Errorf("ansible reload login失败: %v\n", err)
		}
	}
	return nil
}

// UpdateOpenTime 设置指定 game 编号的开服时间。
// 参数:
//
//	num: 要设置开服时间的 game 编号。
//	ipMap: IP 地址到 game 编号列表的映射表。
//	openBookPath: Ansible playbook 文件路径，用于设置开服时间。
//
// 返回值:
//
//	error: 如果获取 IP 或设置开服时间失败，返回错误信息；否则返回 nil。
func UpdateOpenTime(num int, ipMap map[string][]string, openBookPath string) error {
	ip, err := getsomething.GetGameIP(num, ipMap)
	if err != nil {
		return err
	}

	infoLogger.Printf("正在设置开服时间 game 编号为: %d 当前IP为:%s", num, ip)
	cmd := exec.Command("ansible-playbook", "-i", fmt.Sprintf("%s,", ip),
		"-e", fmt.Sprintf("host_name=%s", ip),
		"-e", fmt.Sprintf("area_id=%d", num),
		openBookPath)

	output, err := cmd.CombinedOutput()
	infoLogger.Printf("ansible输出:\n%s\n", output)
	if err != nil {
		return fmt.Errorf("ansible 更新开服时间失败: %v\n", err)
	}
	return nil
}

// UpdateLimit 更新指定 game 编号的限制名单并重载登录服务。
// 参数:
//
//	num: 要更新限制名单的 game 编号。
//	loginSlice: 登录服务器 IP 列表。
//	loginListFilePath: 限制名单文件路径。
//	limitBookPath: Ansible playbook 文件完整路径，用于更新限制名单。
//	loginBookPath: Ansible playbook 文件完整路径，用于重载登录服务。
//
// 返回值:
//
//	error: 如果更新限制名单或重载登录服务失败，返回错误信息；否则返回 nil。
func UpdateLimit(num int, loginSlice []string, loginListFilePath, limitBookPath, loginBookPath string) error {
	for _, loginIP := range loginSlice {
		infoLogger.Printf("正在更新限制名单 服务:%d IP:%s", num, loginIP)
		cmd := exec.Command("ansible-playbook", "-i", fmt.Sprintf("%s,", loginIP),
			"-e", fmt.Sprintf("host_name=%s,", loginIP),
			"-e", fmt.Sprintf("area_id=%d", num),
			"-e", fmt.Sprintf("list_path=%s", loginListFilePath),
			limitBookPath)

		output, err := cmd.CombinedOutput()
		infoLogger.Printf("ansible输出:\n%s\n", output)
		if err != nil {
			return fmt.Errorf("ansible 更新限制名单失败: %v\n", err)
		}
		infoLogger.Printf("正在reload login 当前IP为:%s", loginIP)
		cmd = exec.Command("ansible-playbook",
			"-i", fmt.Sprintf("%s,", loginIP),
			"-e", fmt.Sprintf("host_name=%s,", loginIP),
			loginBookPath)
		output, err = cmd.CombinedOutput()
		infoLogger.Printf("ansible输出:\n%s\n", output)
		if err != nil {
			return fmt.Errorf("ansible reload login失败: %v\n", err)
		}
	}
	return nil
}

// InstallGame 从旧 game 拉取安装包并部署到新 game。
// 参数:
//
//	oldNum: 旧 game 编号，用于拉取安装包。
//	newNum: 新 game 编号，用于部署。
//	bookPath: Ansible playbook 文件所在目录。
//	packageYamlName: 拉取安装包的 playbook 文件名。
//	installYamlName: 部署安装的 playbook 文件名。
//	ipMap: IP 地址到 game 编号列表的映射表。
//	ipGroup: IP 地址到 group ID 的映射表。
//
// 返回值:
//
//	error: 如果拉取安装包、获取 IP、生成模板或部署失败，返回错误信息；否则返回 nil。
func InstallGame(oldNum, newNum int,
	bookPath, packageYamlName, installYamlName string,
	ipMap map[string][]string, ipGroup map[string]int) error {
	oldIP, err := getsomething.GetGameIP(oldNum, ipMap)
	if err != nil {
		return err
	}

	infoLogger.Printf("正在从 game 编号为:%d 拉取最新安装包, IP 为:%s", oldNum, oldIP)
	cmd := exec.Command("ansible-playbook", "-i", fmt.Sprintf("%s,", oldIP),
		"-e", fmt.Sprintf("host_name=%s", oldIP),
		"-e", fmt.Sprintf("area_id=%d", oldNum),
		filepath.Join(bookPath, packageYamlName))

	output, err := cmd.CombinedOutput()
	infoLogger.Printf("ansible输出:\n%s\n", output)
	if err != nil {
		return fmt.Errorf("ansible 拉取最新安装包失败: %v\n", err)
	}

	newIP, err := getsomething.GetGameIP(newNum, ipMap)
	if err != nil {
		return err
	}

	groupID, err := getsomething.GetGroupID(newNum, ipGroup, ipMap)
	if err != nil {
		return err
	}
	if err = CreateVarTemplate(bookPath, newIP, newNum, basePort+newNum, groupID); err != nil {
		return fmt.Errorf("install 模板文件生成失败: %v", err)
	}

	infoLogger.Printf("正在部署 game 编号为:%d, IP 为:%s", newNum, newIP)
	cmd = exec.Command("ansible-playbook", "-i", fmt.Sprintf("%s,", newIP),
		"-e", fmt.Sprintf("host_name=%s", newIP),
		filepath.Join(bookPath, installYamlName))
	output, err = cmd.CombinedOutput()
	infoLogger.Printf("ansible输出:\n%s\n", output)
	if err != nil {
		return fmt.Errorf("ansible 部署失败: %v\n", err)
	}
	return nil
}

// CreateVarTemplate 生成 Ansible 的变量模板文件。
// 参数:
//
//	bookPath: 模板文件所在目录。
//	currentIP: 当前服务器 IP 地址。
//	areaID: game 区域 ID。
//	gamePort: game 服务端口号。
//	groupID: group ID。
//
// 返回值:
//
//	error: 如果读取或写入模板文件失败，返回错误信息；否则返回 nil。
func CreateVarTemplate(bookPath string, currentIP string, areaID, gamePort, groupID int) error {
	// 输入和输出文件路径
	inputFile := filepath.Join(bookPath, "vars", "main.yaml.tmp")
	outputFile := filepath.Join(bookPath, "vars", "main.yaml")

	os.Setenv("currentIP", currentIP)
	os.Setenv("gamePort", strconv.Itoa(gamePort))
	os.Setenv("gameDBName", fmt.Sprintf("cbt4_game_%d", areaID))
	os.Setenv("groupID", strconv.Itoa(groupID))
	os.Setenv("areaID", strconv.Itoa(areaID))

	// 读取文件并替换环境变量
	content, err := envsubst.ReadFile(inputFile)
	if err != nil {
		return fmt.Errorf("读取文件失败:%v", err)
	}

	// 写入文件
	err = os.WriteFile(outputFile, content, 0644)
	if err != nil {
		return fmt.Errorf("写入文件失败:%v", err)
	}
	return nil
}

// QueryCount 执行 SQL 查询并返回单行计数结果。
// 参数:
//
//	db: 数据库连接对象。
//	querySql: 要执行的 SQL 查询语句，通常为返回计数的 SELECT 语句。
//	args: 可变参数，SQL 查询中的占位符参数。
//
// 返回值:
//
//	int: 查询结果的计数值，如果查询失败则返回 0。
//	error: 如果数据库查询或结果扫描失败，返回错误信息；否则返回 nil。
func QueryCount(db *sql.DB, querySql string, args ...interface{}) (int, error) {
	var count int
	err := db.QueryRow(querySql, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("数据库查询错误: %v", err)
	}
	return count, nil
}
