package crack

// SupportedProtocols 是所有支持的爆破协议(由 -m 指定)。
// 已废弃按端口自动识别协议, 协议完全由 -m 决定, 端口任意。
var SupportedProtocols = []string{
	"ftp", "ssh", "wmi", "wmihash", "smb", "mssql",
	"oracle", "mysql", "rdp", "postgres", "redis", "memcached", "mongodb",
}

// IsSupportedProtocol 判断 name 是否为合法协议。
func IsSupportedProtocol(name string) bool {
	for _, p := range SupportedProtocols {
		if p == name {
			return true
		}
	}
	return false
}

var (
	userMap = map[string][]string{
		//"ftp": {"ftp", "admin", "www"},
		"ftp": {"ftp"},
		//"ssh":      {"root", "oracle", "admin"},
		"ssh":       {"root"},
		"wmi":       {"administrator"},
		"wmihash":   {"administrator"},
		"smb":       {"administrator"},
		"mssql":     {"sa"},
		"oracle":    {"oracle", "system"},
		"mysql":     {"root"},
		"rdp":       {"administrator"},
		"postgres":  {"postgres", "admin"},
		"redis":     {""},
		"memcached": {""},
		"mongodb":   {"admin", "root"},
	}

	templatePass = []string{"{user}", "{user}!@#123", "{user}!@#456", "{user}#123", "{user}*PWD", "{user}1", "{user}11", "{user}12#$", "{user}123", "{user}123456", "{user}@111", "{user}@123", "{user}@123#4", "{user}@2016", "{user}@2017", "{user}@2018", "{user}@2019", "{user}@2020", "{user}@2021", "{user}@2022", "{user}_123"}

	commonPass = []string{"", "!QAZ2wsx", "000000", "1", "111111", "123", "123123", "12313", "123321", "1234", "12345!@#$%abc", "123456", "12345678", "123456789", "1234567890", "12345678;abc", "123456Aa", "123qwe!@#", "123qweASD", "1q2w3e", "1qaz2wsx", "1QAZ2wsx", "1qaz@WSX", "1QAZ@WSX", "1qazxsw2", "654321", "666666", "8888888", "a11111", "a123123", "a12345", "a123456", "a123456", "a123456.", "Aa123123", "Aa1234", "Aa1234.", "Aa12345", "Aa12345.", "Aa123456", "Aa123456!", "Aa123456789", "abc+123", "abc123", "abc123456", "abc@123", "admin", "admin123", "Admin123", "admin123!@#", "admin888", "admin@123", "Admin@123", "Admin@1234", "admin@888", "adminadmin", "adminPwd", "Asdfg@123", "Charge123", "P@ssw0rd", "P@ssw0rd!", "P@ssword", "p@ssword", "pass123", "pass@123", "Passw0rd", "password", "qwe123", "qwe123!@#", "root", "sysadmin", "system", "test", "test123", "xcv@123", "zxc1qaz", "Zxcvb123"}
)
