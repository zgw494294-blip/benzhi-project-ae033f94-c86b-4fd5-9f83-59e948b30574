package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
)

type config struct {
	Addr      string
	DBPath    string
	Selfcheck bool
}

func parseConfig(args []string) (config, error) {
	defaultAddr := "127.0.0.1:19081"
	if raw := os.Getenv("PORT"); raw != "" {
		p, err := strconv.Atoi(raw)
		if err != nil || p < 1 || p > 65535 {
			return config{}, fmt.Errorf("PORT 必须是 1-65535 的端口号")
		}
		defaultAddr = net.JoinHostPort("127.0.0.1", raw)
	}
	fs := flag.NewFlagSet("stage-rigging-clearance", flag.ContinueOnError)
	var cfg config
	fs.StringVar(&cfg.Addr, "addr", defaultAddr, "HTTP 监听地址")
	fs.StringVar(&cfg.DBPath, "db", "rigging-clearance.db", "SQLite 数据库路径")
	fs.BoolVar(&cfg.Selfcheck, "selfcheck", false, "执行真实 HTTP 自检后退出")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	host, port, err := net.SplitHostPort(cfg.Addr)
	if err != nil {
		return cfg, fmt.Errorf("-addr 必须是 host:port: %w", err)
	}
	if host == "" {
		return cfg, fmt.Errorf("-addr 不允许省略主机")
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return cfg, fmt.Errorf("-addr 端口无效")
	}
	if cfg.DBPath == "" {
		return cfg, fmt.Errorf("-db 不能为空")
	}
	return cfg, nil
}
