//go:build windows

package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"syscall"
	"time"
	"unsafe"

	"aihelper/internal/ai/extractor"
	"aihelper/internal/app/capture"
	"aihelper/internal/config"
	"aihelper/internal/httpapi"
	"aihelper/internal/repository"
	"aihelper/internal/service"
	"aihelper/internal/source/localjson"
	"aihelper/internal/storage"

	"github.com/atotto/clipboard"
	"github.com/gen2brain/beeep"
	"github.com/tadvi/systray"
	"golang.org/x/sys/windows"
)

const (
	whKeyboardLL   = 13
	hcAction       = 0
	wmKeyDown      = 0x0100
	wmKeyUp        = 0x0101
	wmSysKeyDown   = 0x0104
	wmSysKeyUp     = 0x0105
	vkControl      = 0x11
	vkLControl     = 0xA2
	vkRControl     = 0xA3
	vkC            = 0x43
	doublePressGap = 900 * time.Millisecond
)

var (
	user32                = windows.NewLazySystemDLL("user32.dll")
	procSetWindowsHookEx  = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHook = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx    = user32.NewProc("CallNextHookEx")
	procGetMessage        = user32.NewProc("GetMessageW")
	procTranslateMessage  = user32.NewProc("TranslateMessage")
	procDispatchMessage   = user32.NewProc("DispatchMessageW")
	lastCtrlCPress        time.Time
	ctrlPressed           bool
	appInstance           *desktopApp
	restartWaitPID        = flag.Int("restart-wait", 0, "wait for the given pid before starting")
)

type desktopApp struct {
	db           *sql.DB
	server       *http.Server
	captureSvc   *capture.Service
	solutionSvc  *service.SolutionService
	analysisSvc  *service.CaptureAnalysisService
	settingsSvc  *service.SettingsService
	configPath   string
	webURL       string
	keyboardHook uintptr
	tray         *systray.Systray
	autoMenu     *systray.MenuItem
}

type point struct {
	x int32
	y int32
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
}

type kbdllhookstruct struct {
	vkCode      uint32
	scanCode    uint32
	flags       uint32
	time        uint32
	dwExtraInfo uintptr
}

func main() {
	flag.Parse()
	if *restartWaitPID > 0 {
		waitForPID(*restartWaitPID)
	}

	dbPath := getenv("AIHELPER_DB_PATH", "data/aihelper.db")
	addr := getenv("AIHELPER_ADDR", ":8090")
	configPath := getenv("AIHELPER_CONFIG_PATH", "config/app.json")

	db, err := storage.OpenSQLite(dbPath)
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	repo := repository.NewSQLiteSolutionRepository(db)
	solutionSvc := service.NewSolutionService(repo)
	importSvc := service.NewImportService(solutionSvc, localjson.New())
	captureSvc := capture.NewService(db)
	analysisSvc := service.NewCaptureAnalysisService(captureSvc, solutionSvc, extractor.New(cfg.AI))
	settingsSvc := service.NewSettingsService(configPath)
	handler := httpapi.NewHandler(solutionSvc, importSvc, service.NewCaptureService(captureSvc), analysisSvc, settingsSvc)
	handler.SetAnalysisServiceFactory(func() *service.CaptureAnalysisService {
		freshCfg, err := config.Load(configPath)
		if err != nil {
			log.Printf("reload config for analysis service: %v", err)
			return analysisSvc
		}
		return service.NewCaptureAnalysisService(captureSvc, solutionSvc, extractor.New(freshCfg.AI))
	})

	server := &http.Server{
		Addr:    addr,
		Handler: handler.Routes(),
	}

	appInstance = &desktopApp{
		db:          db,
		server:      server,
		captureSvc:  captureSvc,
		solutionSvc: solutionSvc,
		analysisSvc: analysisSvc,
		settingsSvc: settingsSvc,
		configPath:  configPath,
		webURL:      "http://127.0.0.1" + addr,
	}

	if err := appInstance.installKeyboardHook(); err != nil {
		log.Fatalf("install keyboard hook: %v", err)
	}

	go func() {
		log.Printf("AI Helper listening on %s", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server stopped: %v", err)
		}
	}()

	log.Printf("desktop app is running; tray enabled; double Ctrl+C capture enabled")

	tray, err := systray.New()
	if err != nil {
		log.Fatalf("create systray: %v", err)
	}
	appInstance.tray = tray
	if err := tray.Show(1, "AI Helper"); err != nil {
		log.Printf("show systray: %v", err)
	}
	setupTray(appInstance)
	if err := tray.Run(); err != nil {
		log.Fatalf("systray run: %v", err)
	}
}

func setupTray(app *desktopApp) {
	app.tray.OnRightClick(func() {
		log.Printf("tray right click")
	})
	app.tray.OnClick(func() {
		log.Printf("tray left click")
	})
	app.tray.AppendMenu("打开知识库", func() {
		openBrowser(app.webURL)
	})
	app.tray.AppendMenu("查看最近抓取", func() {
		openBrowser(app.webURL + "#pending-section")
	})
	app.tray.AppendSeparator()
	app.tray.AppendMenu("自动分析: 加载中", func() {
		go app.toggleAutoAnalyze()
	})
	app.autoMenu = app.tray.Menu[len(app.tray.Menu)-1]
	app.refreshAutoAnalyzeMenuLabel()
	app.tray.AppendSeparator()
	app.tray.AppendMenu("重启", func() {
		go app.restart()
	})
	app.tray.AppendSeparator()
	app.tray.AppendMenu("关闭", func() {
		go app.shutdown(false)
	})
}

func (a *desktopApp) refreshAutoAnalyzeMenuLabel() {
	if a.autoMenu == nil {
		return
	}
	settings, err := a.settingsSvc.Get()
	if err != nil {
		log.Printf("load settings for tray: %v", err)
		a.autoMenu.Label = "自动分析: 读取失败"
		return
	}
	if settings.Capture.AutoAnalyze {
		a.autoMenu.Label = "关闭自动分析"
		return
	}
	a.autoMenu.Label = "开启自动分析"
}

func (a *desktopApp) toggleAutoAnalyze() {
	settings, err := a.settingsSvc.Get()
	if err != nil {
		log.Printf("load settings for toggle: %v", err)
		return
	}
	var input service.SettingsDTO
	input.Capture.AutoAnalyze = !settings.Capture.AutoAnalyze
	updated, err := a.settingsSvc.Update(input)
	if err != nil {
		log.Printf("update settings for toggle: %v", err)
		return
	}
	a.refreshAutoAnalyzeMenuLabel()
	if updated.Capture.AutoAnalyze {
		notify("自动分析", "已开启")
	} else {
		notify("自动分析", "已关闭")
	}
}

func (a *desktopApp) installKeyboardHook() error {
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		hookCallback := windows.NewCallback(keyboardProc)
		hook, _, err := procSetWindowsHookEx.Call(
			uintptr(whKeyboardLL),
			hookCallback,
			0,
			0,
		)
		if hook == 0 {
			ready <- err
			return
		}
		a.keyboardHook = hook
		ready <- nil
		runKeyboardMessageLoop()
	}()
	return <-ready
}

func (a *desktopApp) restart() {
	exe, err := os.Executable()
	if err != nil {
		log.Printf("resolve exe for restart: %v", err)
		return
	}
	cmd := exec.Command(exe, "--restart-wait", strconv.Itoa(os.Getpid()))
	cmd.Dir = mustGetwd()
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		log.Printf("restart spawn failed: %v", err)
		return
	}
	a.shutdown(true)
}

func (a *desktopApp) shutdown(fromRestart bool) {
	if a.keyboardHook != 0 {
		procUnhookWindowsHook.Call(a.keyboardHook)
		a.keyboardHook = 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = a.server.Shutdown(ctx)
	_ = a.db.Close()
	if a.tray != nil {
		_ = a.tray.Stop()
	}
	if !fromRestart {
		os.Exit(0)
	}
	os.Exit(0)
}

func keyboardProc(code int, wParam uintptr, lParam uintptr) uintptr {
	if code == hcAction {
		kbd := (*kbdllhookstruct)(unsafe.Pointer(lParam))
		switch wParam {
		case wmKeyDown, wmSysKeyDown:
			if isCtrlKey(kbd.vkCode) {
				ctrlPressed = true
			}
			if kbd.vkCode == vkC && ctrlPressed {
				now := time.Now()
				if !lastCtrlCPress.IsZero() && now.Sub(lastCtrlCPress) <= doublePressGap {
					log.Printf("double Ctrl+C detected, gap=%s", now.Sub(lastCtrlCPress).Truncate(time.Millisecond))
					lastCtrlCPress = time.Time{}
					go appInstance.handleConfirmedCapture()
				} else {
					lastCtrlCPress = now
					log.Printf("first Ctrl+C detected")
				}
			}
		case wmKeyUp, wmSysKeyUp:
			if isCtrlKey(kbd.vkCode) {
				ctrlPressed = false
			}
		}
	}
	ret, _, _ := procCallNextHookEx.Call(0, uintptr(code), wParam, lParam)
	return ret
}

func runKeyboardMessageLoop() {
	var message msg
	for {
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		switch int32(ret) {
		case -1:
			log.Println("keyboard message loop error")
			return
		case 0:
			return
		default:
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
			procDispatchMessage.Call(uintptr(unsafe.Pointer(&message)))
		}
	}
}

func isCtrlKey(vk uint32) bool {
	return vk == vkControl || vk == vkLControl || vk == vkRControl
}

func (a *desktopApp) handleConfirmedCapture() {
	time.Sleep(300 * time.Millisecond)

	text, err := clipboard.ReadAll()
	if err != nil {
		log.Printf("read clipboard: %v", err)
		notify("抓取失败", "无法读取剪贴板内容")
		return
	}
	log.Printf("clipboard read after double Ctrl+C, chars=%d", len([]rune(text)))

	result, err := a.captureSvc.CaptureClipboard(context.Background(), text)
	if err != nil {
		if errors.Is(err, capture.ErrEmptyClipboard) {
			notify("抓取失败", "剪贴板里没有文本内容")
			return
		}
		log.Printf("capture clipboard: %v", err)
		notify("抓取失败", "保存抓取内容时出错")
		return
	}

	if result.Duplicate {
		log.Printf("capture duplicate id=%d type=%s chars=%d", result.ID, result.ContentType, result.CharCount)
		notify("已抓取", formatDuplicateMessage(result))
		return
	}

	log.Printf("capture saved id=%d type=%s chars=%d lines=%d", result.ID, result.ContentType, result.CharCount, result.LineCount)
	notify("已经抓取", formatSuccessMessage(result))
	a.tryAutoAnalyze(result.ID)
}

func (a *desktopApp) tryAutoAnalyze(captureID int64) {
	cfg, err := config.Load(a.configPath)
	if err != nil {
		log.Printf("load config for auto analyze: %v", err)
		return
	}
	if !cfg.Capture.AutoAnalyze {
		return
	}

	analysisSvc := service.NewCaptureAnalysisService(a.captureSvc, a.solutionSvc, extractor.New(cfg.AI))

	_, err = analysisSvc.AnalyzeCapture(context.Background(), captureID)
	if err != nil {
		if errors.Is(err, service.ErrAnalysisRecentlyRequested) {
			log.Printf("auto analyze skipped capture=%d: recent duplicate analysis request", captureID)
			return
		}
		if errors.Is(err, service.ErrAnalysisTimedOut) {
			log.Printf("auto analyze timed out capture=%d", captureID)
			notify("自动分析超时", "AI 分析超过 15 秒，片段已保存")
			return
		}
		log.Printf("auto analyze failed capture=%d: %v", captureID, err)
		notify("自动分析失败", "片段已保存，但 AI 分析失败")
		return
	}

	_, err = analysisSvc.PromoteCapture(context.Background(), captureID)
	if err != nil {
		log.Printf("auto promote failed capture=%d: %v", captureID, err)
		notify("自动分析完成", "已分析，但沉淀为知识时失败")
		return
	}

	log.Printf("auto analyzed and promoted capture=%d", captureID)
	notify("自动分析完成", "已自动沉淀到知识库")
}

func formatSuccessMessage(result *capture.Result) string {
	return "已保存到本地库，类型: " + result.ContentType +
		"，字符数: " + strconv.Itoa(result.CharCount) +
		"，行数: " + strconv.Itoa(result.LineCount)
}

func formatDuplicateMessage(result *capture.Result) string {
	return "内容之前已抓取过，类型: " + result.ContentType +
		"，字符数: " + strconv.Itoa(result.CharCount)
}

func notify(title, message string) {
	if err := beeep.Notify(title, message, ""); err != nil {
		log.Printf("notify: %v", err)
	}
}

func waitForPID(pid int) {
	for i := 0; i < 100; i++ {
		process, err := os.FindProcess(pid)
		if err != nil {
			return
		}
		if err := process.Signal(syscall.Signal(0)); err != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func openBrowser(url string) {
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Start()
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
