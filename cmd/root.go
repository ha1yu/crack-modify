package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"crack-modify/internal/utils"
	"crack-modify/pkg/crack"

	"github.com/projectdiscovery/gologger"
	"github.com/projectdiscovery/gologger/formatter"
	"github.com/projectdiscovery/gologger/levels"
	"github.com/projectdiscovery/gologger/writer"
	"github.com/spf13/cobra"
)

// CommonOptions 通用选项(输入/输出)
type CommonOptions struct {
	Input     string
	InputFile string

	OutputFile string
	ResultFile string

	NoColor bool
	Debug   bool
}

// CrackOptions 爆破选项
type CrackOptions struct {
	Module   string
	User     string
	Pass     string
	UserFile string
	PassFile string

	Threads  int
	Timeout  int
	Delay    int
	CrackAll bool
	Spray    bool
}

var (
	commonOptions CommonOptions
	crackOptions  CrackOptions
	targets       []string
	userDict      []string
	passDict      []string
)

var rootCmd = &cobra.Command{
	Use:               "crack-modify",
	Short:             "一个有点好用的弱口令爆破工具",
	Long:              "常见服务弱口令爆破,支持ftp,ssh,wmi,wmihash,smb,mssql,oracle,mysql,rdp,postgres,redis,memcached,mongodb",
	CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		commonOptions.configureOutput()
		if err := commonOptions.validateOptions(); err != nil {
			return err
		}
		if err := commonOptions.configureOptions(); err != nil {
			return err
		}
		if err := crackOptions.validateOptions(); err != nil {
			return err
		}
		if err := crackOptions.configureOptions(); err != nil {
			return err
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return crackOptions.run()
	},
	SilenceUsage:  true,
	SilenceErrors: false,
}

// validateOptions 验证输入选项
func (o *CommonOptions) validateOptions() error {
	if o.Input == "" && o.InputFile == "" {
		return fmt.Errorf("未提供输入, 使用 -i 指定单个目标或 -f 指定目标文件")
	}
	if o.InputFile != "" && !utils.FileExists(o.InputFile) {
		return fmt.Errorf("文件 %v 不存在", o.InputFile)
	}
	return nil
}

// configureOutput 配置输出
func (o *CommonOptions) configureOutput() {
	if o.NoColor {
		gologger.DefaultLogger.SetFormatter(formatter.NewCLI(true))
	}
	if o.Debug {
		gologger.DefaultLogger.SetMaxLevel(levels.LevelDebug)
	}
	gologger.DefaultLogger.SetWriter(writer.NewLogFile(o.OutputFile))
}

// configureOptions 加载目标
func (o *CommonOptions) configureOptions() error {
	if o.Input != "" {
		targets = append(targets, o.Input)
	} else {
		lines, err := utils.ReadLines(o.InputFile)
		if err != nil {
			return err
		}
		targets = append(targets, lines...)
	}
	targets = utils.RemoveDuplicate(targets)

	opt, _ := json.Marshal(o)
	gologger.Debug().Msgf("commonOptions: %v", string(opt))
	return nil
}

// validateOptions 验证爆破选项
func (o *CrackOptions) validateOptions() error {
	if o.UserFile != "" && !utils.FileExists(o.UserFile) {
		return fmt.Errorf("文件 %v 不存在", o.UserFile)
	}
	if o.PassFile != "" && !utils.FileExists(o.PassFile) {
		return fmt.Errorf("文件 %v 不存在", o.PassFile)
	}
	return nil
}

// configureOptions 加载字典
func (o *CrackOptions) configureOptions() error {
	var err error
	if o.User != "" {
		userDict = strings.Split(o.User, ",")
	}
	if o.Pass != "" {
		passDict = strings.Split(o.Pass, ",")
	}
	if o.UserFile != "" {
		if userDict, err = utils.ReadLines(o.UserFile); err != nil {
			return err
		}
	}
	if o.PassFile != "" {
		if passDict, err = utils.ReadLines(o.PassFile); err != nil {
			return err
		}
	}
	// 未指定字典时使用内置默认字典(由 crack.NewRunner 注入)
	userDict = utils.RemoveDuplicate(userDict)
	passDict = utils.RemoveDuplicate(passDict)

	opt, _ := json.Marshal(o)
	gologger.Debug().Msgf("crackOptions: %v", string(opt))
	gologger.Debug().Msgf("userDict: %v", len(userDict))
	gologger.Debug().Msgf("passDict: %v", len(passDict))
	return nil
}

// run 执行爆破
func (o *CrackOptions) run() error {
	start := time.Now()
	options := &crack.Options{
		Threads:  o.Threads,
		Timeout:  o.Timeout,
		Delay:    o.Delay,
		CrackAll: o.CrackAll,
		Spray:    o.Spray,
		// 留空则 NewRunner 自动注入内置的 UserMap/CommonPass/TemplatePass
	}
	crackRunner, err := crack.NewRunner(options)
	if err != nil {
		return fmt.Errorf("crack.NewRunner() err, %w", err)
	}
	addrs := crack.ParseTargets(targets)
	addrs = crack.FilterModule(addrs, o.Module)
	if len(addrs) == 0 {
		gologger.Error().Msgf("目标为空")
		return nil
	}
	// 存活探测
	gologger.Info().Msgf("存活探测")
	addrs = crackRunner.CheckAlive(addrs)
	gologger.Info().Msgf("存活数量: %v", len(addrs))
	// 服务爆破
	results := crackRunner.Run(addrs, userDict, passDict)
	if len(results) > 0 {
		gologger.Info().Msgf("爆破成功: %v", len(results))
		for _, result := range results {
			gologger.Print().Msgf("%v -> %v %v", result.Protocol, result.Addr, result.UserPass)
		}
	}
	// 保存结果
	if commonOptions.ResultFile != "" {
		err = utils.SaveMarshal(commonOptions.ResultFile, results)
		if err != nil {
			gologger.Error().Msgf("utils.SaveMarshal() err, %v", err)
		}
	}
	gologger.Info().Msgf("运行时间: %v", time.Since(start))
	return nil
}

// Execute 注册全部 flag 并执行根命令。
func Execute() {
	// 输入/输出 flag
	rootCmd.Flags().StringVarP(&commonOptions.Input, "input", "i", "", "单个目标(例: -i '127.0.0.1:3306' 或 '192.168.1.0/24:445')")
	rootCmd.Flags().StringVarP(&commonOptions.InputFile, "input-file", "f", "", "目标文件,每行一个(例: -f 'targets.txt')")
	rootCmd.Flags().StringVar(&commonOptions.ResultFile, "result", "", "命中结果导出文件(JSON)")
	rootCmd.Flags().StringVarP(&commonOptions.OutputFile, "output", "o", "result.txt", "日志与结果输出文件")
	rootCmd.Flags().BoolVar(&commonOptions.NoColor, "no-color", false, "关闭彩色输出")
	rootCmd.Flags().BoolVar(&commonOptions.Debug, "debug", false, "显示调试输出")

	// 爆破 flag
	rootCmd.Flags().StringVarP(&crackOptions.Module, "module", "m", "all", "指定爆破模块(ftp,ssh,wmi,wmihash,smb,mssql,oracle,mysql,rdp,postgres,redis,memcached,mongodb)")
	rootCmd.Flags().StringVar(&crackOptions.User, "user", "", "用户名,逗号分隔(例: --user 'admin,root')")
	rootCmd.Flags().StringVar(&crackOptions.Pass, "pass", "", "口令,逗号分隔(例: --pass 'admin,root')")
	rootCmd.Flags().StringVar(&crackOptions.UserFile, "user-file", "", "用户名字典文件(例: --user-file 'user.txt')")
	rootCmd.Flags().StringVar(&crackOptions.PassFile, "pass-file", "", "口令字典文件(例: --pass-file 'pass.txt')")
	rootCmd.Flags().IntVar(&crackOptions.Threads, "threads", 1, "并发线程数")
	rootCmd.Flags().IntVar(&crackOptions.Timeout, "timeout", 10, "单次连接超时(秒)")
	rootCmd.Flags().IntVar(&crackOptions.Delay, "delay", 0, "请求间隔(秒, 0 关闭限速)")
	rootCmd.Flags().BoolVar(&crackOptions.CrackAll, "crack-all", false, "命中后继续爆破该目标的全部口令")
	rootCmd.Flags().BoolVar(&crackOptions.Spray, "spray", false, "密码喷洒模式: 每个口令先遍历全部用户再换下一个(防账户锁定, 配合 --delay)")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
