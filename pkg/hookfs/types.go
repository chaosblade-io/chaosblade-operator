package hookfs

var defaultHookPoints = []string{
	"read",
	"write",
	"mkdir",
	"rmdir",
	"opendir",
	"fsync",
	"flush",
	"release",
	"truncate",
	"getattr",
	"chown",
	"utimens",
	"allocate",
	"getlk",
	"setlk",
	"setlkw",
	"statfs",
	"readlink",
	"symlink",
	"create",
	"access",
	"link",
	"mknod",
	"rename",
	"unlink",
	"getxattr",
	"listxattr",
	"removexattr",
	"setxattr",
}

var (
	InjectPath  = "/inject"
	RecoverPath = "/recover"
)
