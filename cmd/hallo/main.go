package main

import (
	"flag"
	"fmt"
	iofs "io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"

	"hallo/internal/api"
	"hallo/internal/auth"
	"hallo/internal/config"
	"hallo/internal/db"
	"hallo/internal/models"
	"hallo/internal/web"
	"hallo/internal/xray"
)

// version is set at release build time: -ldflags "-X main.version=v0.1.0"
var version = "dev"

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("hallo ")
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve":
		serve(os.Args[2:])
	case "plan":
		planCmd(os.Args[2:])
	case "user":
		userCmd(os.Args[2:])
	case "xray":
		xrayCmd(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Println(version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "未知命令：%s\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Hallo 面板 %s（第 1 期：单节点 + Xray）

用法：
  hallo serve [--listen :18080] [--data data]
  hallo plan add --name NAME [--limit 0] [--days 0] [--note TEXT]
  hallo plan list
  hallo user add --email ID [--plan NAME] [--remark TEXT]
  hallo user list
  hallo xray reload
  hallo version

环境变量：
  HALLO_LISTEN  面板监听地址（默认 :18080）
  HALLO_DATA    数据目录（默认 ./data）
  HALLO_XRAY    xray 可执行文件路径
  HALLO_DEV=1   开发模式（默认入站端口 18443）
`, version)
}

func openDB(cfg config.Config) *db.DB {
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatal(err)
	}
	database, err := db.Open(cfg.DBPath())
	if err != nil {
		log.Fatal(err)
	}
	return database
}

func serve(args []string) {
	cfg := config.Default()
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	fs.StringVar(&cfg.Listen, "listen", cfg.Listen, "面板监听地址")
	fs.StringVar(&cfg.DataDir, "data", cfg.DataDir, "数据目录")
	_ = fs.Parse(args)

	database := openDB(cfg)
	defer database.Close()
	if cfg.PublicURL != "" && database.GetSetting("public_url", "") == "" {
		_ = database.SetSetting("public_url", cfg.PublicURL)
	}

	bin := database.GetSetting("xray_path", xray.DefaultBin(cfg.DataDir))
	xm := xray.New(bin, cfg.XrayConfigPath())
	webFS, err := iofs.Sub(web.Dist, "dist")
	if err != nil {
		log.Fatal(err)
	}
	srv := api.New(cfg, database, xm, webFS, version)

	in, err := database.GetInbound()
	if err == nil && in.PrivateKey != "" && in.PrivateKey != "CHANGE_ME_PRIVATE" {
		users, _ := database.ActiveUsers()
		_ = xm.WriteConfig(*in, users)
		if _, statErr := os.Stat(bin); statErr == nil {
			if err := xm.Reload(); err != nil {
				log.Printf("启动 xray 失败：%v", err)
			}
		}
	}

	httpSrv := &http.Server{Addr: cfg.Listen, Handler: srv.Router()}
	go func() {
		log.Printf("面板监听 %s  数据目录 %s", cfg.Listen, cfg.DataDir)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	xm.Stop()
	_ = httpSrv.Close()
}

func planCmd(args []string) {
	if len(args) < 1 {
		log.Fatal("用法：hallo plan add|list")
	}
	cfg := config.Default()
	database := openDB(cfg)
	defer database.Close()
	switch args[0] {
	case "list":
		items, err := database.ListPlans()
		if err != nil {
			log.Fatal(err)
		}
		if len(items) == 0 {
			fmt.Println("(空)")
			return
		}
		for _, p := range items {
			fmt.Printf("%d\t%s\tlimit=%d\tdays=%d\t%s\n", p.ID, p.Name, p.TrafficLimit, p.DurationDays, p.Note)
		}
	case "add":
		fs := flag.NewFlagSet("plan add", flag.ExitOnError)
		name := fs.String("name", "", "套餐名")
		limit := fs.String("limit", "0", "流量上限（字节，或 10g / 100m；0 不限）")
		days := fs.Int("days", 0, "有效天数，0 不限")
		note := fs.String("note", "", "备注")
		_ = fs.Parse(args[1:])
		if strings.TrimSpace(*name) == "" {
			log.Fatal("--name 必填")
		}
		n, err := xray.ParseLimitBytes(*limit)
		if err != nil {
			log.Fatal(err)
		}
		id, err := database.CreatePlan(models.Plan{Name: *name, TrafficLimit: n, DurationDays: *days, Note: *note})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("已创建套餐 id=%d name=%s\n", id, *name)
	default:
		log.Fatal("用法：hallo plan add|list")
	}
}

func userCmd(args []string) {
	if len(args) < 1 {
		log.Fatal("用法：hallo user add|list")
	}
	cfg := config.Default()
	database := openDB(cfg)
	defer database.Close()
	switch args[0] {
	case "list":
		items, err := database.ListUsers()
		if err != nil {
			log.Fatal(err)
		}
		if len(items) == 0 {
			fmt.Println("(空)")
			return
		}
		for _, u := range items {
			en := "off"
			if u.Enabled {
				en = "on"
			}
			fmt.Printf("%d\t%s\t%s\t%s\t%s\n", u.ID, u.Email, u.UUID, u.PlanName, en)
		}
	case "add":
		fs := flag.NewFlagSet("user add", flag.ExitOnError)
		email := fs.String("email", "", "用户标识 / 邮箱")
		planName := fs.String("plan", "", "套餐名")
		remark := fs.String("remark", "", "备注")
		_ = fs.Parse(args[1:])
		if strings.TrimSpace(*email) == "" {
			log.Fatal("--email 必填")
		}
		u := models.User{
			Email:   strings.TrimSpace(*email),
			Remark:  *remark,
			UUID:    uuid.NewString(),
			Enabled: true,
		}
		tok, err := auth.RandomToken(16)
		if err != nil {
			log.Fatal(err)
		}
		u.SubToken = tok
		if strings.TrimSpace(*planName) != "" {
			p, err := database.GetPlanByName(*planName)
			if err != nil {
				log.Fatal("找不到套餐：", *planName)
			}
			u.PlanID = &p.ID
			if p.DurationDays > 0 {
				t := time.Now().Add(time.Duration(p.DurationDays) * 24 * time.Hour)
				u.ExpireAt = &t
			}
		}
		id, err := database.CreateUser(u)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("已创建用户 id=%d email=%s uuid=%s\n", id, u.Email, u.UUID)
	default:
		log.Fatal("用法：hallo user add|list")
	}
}

func xrayCmd(args []string) {
	if len(args) < 1 || args[0] != "reload" {
		log.Fatal("用法：hallo xray reload")
	}
	cfg := config.Default()
	database := openDB(cfg)
	defer database.Close()
	in, err := database.GetInbound()
	if err != nil {
		log.Fatal("尚未配置入站，请先打开面板完成初始化")
	}
	users, err := database.ActiveUsers()
	if err != nil {
		log.Fatal(err)
	}
	bin := database.GetSetting("xray_path", xray.DefaultBin(cfg.DataDir))
	xm := xray.New(bin, cfg.XrayConfigPath())
	if err := xm.WriteConfig(*in, users); err != nil {
		log.Fatal(err)
	}
	if err := xm.Reload(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("xray 已重载")
}
