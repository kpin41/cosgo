package app

import (
	"cosgo/logger"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//其他模块可以使用pflag设置额外的参数
var Flag *viper.Viper
var Config *viper.Viper

func init() {
	Flag = viper.New()
	Config = viper.New()
	pflag.Bool("debug", false, "developer model")
	pflag.String("logdir", "", "app logs dir")
	pflag.String("pidfile", "", "app pid file")
	pflag.String("profile", "", "profile address")
	var (
		tmpDir      string
		appName     string
		appBinDir   string
		appWorkDir  string
		appExecFile string
	)

	appWorkDir, _ = os.Getwd()
	appExecFile, _ = exec.LookPath(os.Args[0])
	tmpDir, appName = filepath.Split(appExecFile)

	if filepath.IsAbs(appExecFile) {
		appBinDir = filepath.Dir(tmpDir)
		appWorkDir = filepath.Dir(appBinDir)
	} else {
		appBinDir = filepath.Join(appWorkDir, filepath.Dir(appExecFile))
		appWorkDir, _ = filepath.Split(appBinDir)
		appWorkDir = filepath.Dir(appWorkDir)
	}

	ext := filepath.Ext(appExecFile)
	if ext != "" {
		appName = strings.TrimRight(appName, ext)
	}

	Flag.SetDefault("name", appName)
	Flag.SetDefault("logdir", filepath.Join(appWorkDir, "logs"))
	Flag.SetDefault("pidfile", filepath.Join(appBinDir, appName+".pid"))
	Flag.SetDefault("appBinDir", appBinDir)
	Flag.SetDefault("appWorkDir", appWorkDir)
	Flag.SetDefault("appExecFile", appExecFile)

}

func initFlag() error {
	pflag.Parse()
	Flag.BindPFlags(pflag.CommandLine)
	//设置日志
	logdir := Flag.GetString("logdir")
	if logdir != "" {
		logger.SetLogPathTrim(Flag.GetString(""))
	}

	Debug = Flag.GetBool("debug")
	return nil
}
