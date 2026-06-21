package cmd

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// 集成测试通过编译真实二进制并 exec 运行来验证端到端 CLI 行为。
// 这些测试较慢(需编译), 在 -short 模式下跳过。

var (
	binaryPath string
	buildErr   error
)

func TestMain(m *testing.M) {
	flag.Parse()
	// 在 -short 模式下直接跳过集成测试(无需编译二进制)
	if testing.Short() {
		os.Exit(m.Run())
	}
	// 预编译二进制到临时路径, 所有用例共享
	tmp, err := os.MkdirTemp("", "crack-modify-itest-*")
	if err != nil {
		os.Exit(1)
	}
	binaryPath = filepath.Join(tmp, "crack-modify-test")
	// 从模块根目录构建 main 包(测试运行在 cmd/ 子目录, "." 会指向 cmd 包而非 main)。
	// cmd 包的源码路径是 .../cmd, 模块根是其上一级。
	buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
	// 通过 CWD 切到模块根: 测试编译时 runtime 文件在 cmd/, 上一级即模块根。
	if cwd, err := os.Getwd(); err == nil {
		buildCmd.Dir = filepath.Dir(cwd)
	}
	buildErr = buildCmd.Run()
	// 即使编译失败也运行 m, 让各用例报告具体错误
	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

// ensureBinary 在每个用例开始前确认二进制已构建。
func ensureBinary(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	if buildErr != nil {
		t.Fatalf("failed to build test binary: %v", buildErr)
	}
}

// runBinary 执行已编译的二进制, 返回 stdout+stderr、退出码。
func runBinary(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	// -o 默认写 result.txt 到 CWD, 指向临时目录避免污染仓库
	cmd.Dir = t.TempDir()
	err := cmd.Run()
	exitCode := 0
	if ee, ok := err.(*exec.ExitError); ok {
		exitCode = ee.ExitCode()
	} else if err != nil {
		// 非退出错误(如二进制不存在)
		t.Fatalf("exec failed: %v", err)
	}
	return out.String(), exitCode
}

// TestCLIRootHelp 验证根命令 -h 输出(现已合并全部爆破 flag, 无需 crack 子命令)。
func TestCLIRootHelp(t *testing.T) {
	ensureBinary(t)
	out, code := runBinary(t, "-h")
	if code != 0 {
		t.Errorf("-h exit code = %d, want 0", code)
	}
	if !strings.Contains(out, "弱口令爆破") {
		t.Errorf("-h missing tool description; got:\n%s", out)
	}
	// 13 协议应出现在 -h
	wantProtocols := []string{
		"ftp", "ssh", "wmi", "wmihash", "smb", "mssql",
		"oracle", "mysql", "rdp", "postgres", "redis", "memcached", "mongodb",
	}
	for _, p := range wantProtocols {
		if !strings.Contains(out, p) {
			t.Errorf("-h missing protocol %q", p)
		}
	}
	// 全部 flag 应出现在 -h(不再有子命令, flag 直接挂在根命令)
	wantFlags := []string{
		"--module", "--user", "--pass", "--user-file", "--pass-file",
		"--threads", "--timeout", "--delay", "--crack-all", "--spray",
		"--input", "--input-file", "--result", "--output",
	}
	for _, f := range wantFlags {
		if !strings.Contains(out, f) {
			t.Errorf("-h missing flag %q", f)
		}
	}
}

// TestCLINoInput 验证无输入参数时报错退出。
func TestCLINoInput(t *testing.T) {
	ensureBinary(t)
	// 无 -i/-f 直接运行 → validateOptions 报错, 非零退出
	_, code := runBinary(t)
	if code == 0 {
		t.Error("without input should exit non-zero")
	}
}

// TestCLINonexistentUserFile 验证 --user-file 指向不存在文件时报错退出。
func TestCLINonexistentUserFile(t *testing.T) {
	ensureBinary(t)
	out, code := runBinary(t, "-i", "127.0.0.1:3306", "-m", "mysql", "--user-file", "/no/such/file.txt")
	if code == 0 {
		t.Error("crack with missing --user-file should exit non-zero")
	}
	if !strings.Contains(out, "不存在") {
		t.Errorf("expected '不存在' error; got:\n%s", out)
	}
}

// TestCLINonexistentInputFile 验证 -f 指向不存在文件时报错退出。
func TestCLINonexistentInputFile(t *testing.T) {
	ensureBinary(t)
	_, code := runBinary(t, "-f", "/no/such/targets.txt")
	if code == 0 {
		t.Error("crack with missing -f should exit non-zero")
	}
}

// TestCLICrackDeadPortNoCrash 验证对一个连不上的目标完整跑完不 panic。
// 协议由 -m 指定(端口任意), 端口 1 几乎必然无服务 → 存活数量 0。1s timeout 避免长时间阻塞。
func TestCLICrackDeadPortNoCrash(t *testing.T) {
	ensureBinary(t)
	out, code := runBinary(t, "-i", "127.0.0.1:1", "-m", "mysql",
		"--user", "root", "--pass", "wrongpass", "--timeout", "1", "--threads", "1")
	if code != 0 {
		t.Errorf("crack dead port exit code = %d, want 0; out:\n%s", code, out)
	}
	if !strings.Contains(out, "存活数量: 0") {
		t.Errorf("expected 0 alive; got:\n%s", out)
	}
}

// TestCLIMissingModule 验证 -m 必填: 不传 -m 应报错非零退出。
func TestCLIMissingModule(t *testing.T) {
	ensureBinary(t)
	out, code := runBinary(t, "-i", "127.0.0.1:3306", "--user", "root", "--pass", "x")
	if code == 0 {
		t.Error("without -m should exit non-zero")
	}
	if !strings.Contains(out, "必须用 -m") {
		t.Errorf("expected '必须用 -m' error; got:\n%s", out)
	}
}

// TestCLIInvalidModule 验证 -m 传非法协议时报错非零。
func TestCLIInvalidModule(t *testing.T) {
	ensureBinary(t)
	out, code := runBinary(t, "-i", "127.0.0.1:3306", "-m", "notaproto", "--user", "root", "--pass", "x")
	if code == 0 {
		t.Error("invalid -m should exit non-zero")
	}
	if !strings.Contains(out, "不支持的模块") {
		t.Errorf("expected '不支持的模块' error; got:\n%s", out)
	}
}

// TestCLILegacyProtoSyntaxSkipped 验证旧的 |协议 语法已废弃:
// -i 'ip:port|proto' 现在被解析为非法端口(含 |) → 目标为空。
func TestCLILegacyProtoSyntaxSkipped(t *testing.T) {
	ensureBinary(t)
	out, code := runBinary(t, "-i", "127.0.0.1:3306|mysql", "-m", "mysql",
		"--user", "root", "--pass", "x", "--timeout", "1")
	// 目标被跳过 → "目标为空", 但程序正常退出 exit 0
	if code != 0 {
		t.Errorf("legacy syntax exit code = %d, want 0; out:\n%s", code, out)
	}
	if !strings.Contains(out, "目标为空") {
		t.Errorf("expected '目标为空' for legacy |proto syntax; got:\n%s", out)
	}
}

// TestCLICrackMixedTargetFile 验证目标文件解析 + 存活探测进度数正确。
// 协议由 -m 统一指定, 目标文件只放 ip:port(含 CIDR/段/逗号)。
func TestCLICrackMixedTargetFile(t *testing.T) {
	ensureBinary(t)
	// 写入多种 ip 格式(端口不同, 但协议都由 -m mysql 指定)
	targets := []string{
		"127.0.0.1:3306",       // 单 ip 标准端口
		"127.0.0.1:3307",       // 单 ip 非标准端口(协议仍由 -m 定)
		"127.0.0.1,127.0.0.2:3306", // 逗号列表
		"127.0.0.1-2:3306",     // IP 段
	}
	dir := t.TempDir()
	tf := filepath.Join(dir, "targets.txt")
	content := strings.Join(targets, "\n") + "\n"
	if err := os.WriteFile(tf, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	out, code := runBinary(t, "-f", tf, "-m", "mysql",
		"--user", "root", "--pass", "x", "--timeout", "1", "--threads", "4")
	if code != 0 {
		t.Errorf("mixed file exit code = %d, want 0; out:\n%s", code, out)
	}
	// 展开后: 3306(1) + 3307(1) + 逗号列表(2) + IP段(2) = 6 个目标
	if !strings.Contains(out, "6 / 6") {
		t.Errorf("expected progress '6 / 6' (all targets parsed); got:\n%s", out)
	}
}

// TestCLIResultFileExport 验证 --result 导出 JSON 结果文件(无命中时为 "null")。
func TestCLIResultFileExport(t *testing.T) {
	ensureBinary(t)
	dir := t.TempDir()
	resultFile := filepath.Join(dir, "found.json")
	// 协议由 -m 指定 + 死端口, 确保 ParseTargets 产出目标并走到结果保存阶段
	cmd := exec.Command(binaryPath, "-i", "127.0.0.1:1", "-m", "mysql",
		"--user", "root", "--pass", "x", "--timeout", "1",
		"--result", resultFile)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		// 死端口无命中也是正常退出(exit 0); 只有非零才视为失败
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() != 0 {
			t.Fatalf("crack failed: %v\n%s", err, out.String())
		}
	}
	// 无命中时 SaveMarshal 写入 "null" (空 slice 的 JSON), 文件应存在
	data, err := os.ReadFile(resultFile)
	if err != nil {
		t.Fatalf("result file not created: %v", err)
	}
	// 空结果 marshalled 为 "null", 非空为 JSON 数组
	s := strings.TrimSpace(string(data))
	if s != "null" && !strings.HasPrefix(s, "[") {
		t.Errorf("result file content = %q, want 'null' or JSON array", s)
	}
}
