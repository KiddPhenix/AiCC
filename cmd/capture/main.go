//go:build windows

package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"os"
	"runtime"
	"strconv"
	"time"
	"unsafe"

	"aihelper/internal/ai/extractor"
	"aihelper/internal/app/capture"
	"aihelper/internal/config"
	"aihelper/internal/repository"
	"aihelper/internal/service"
	"aihelper/internal/storage"

	"github.com/atotto/clipboard"
	"github.com/gen2brain/beeep"
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
	user32                 = windows.NewLazySystemDLL("user32.dll")
	procSetWindowsHookEx   = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHook  = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx     = user32.NewProc("CallNextHookEx")
	procGetMessage         = user32.NewProc("GetMessageW")
	procTranslateMessage   = user32.NewProc("TranslateMessage")
	procDispatchMessage    = user32.NewProc("DispatchMessageW")
	lastCtrlCPress         time.Time
	ctrlPressed            bool
	captureServiceInstance *capture.Service
	dbInstance             *sql.DB
)

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
	dbPath := getenv("AIHELPER_DB_PATH", "data/aihelper.db")
	db, err := storage.OpenSQLite(dbPath)
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	dbInstance = db

	captureServiceInstance = capture.NewService(db)

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
		log.Fatalf("install keyboard hook: %v", err)
	}
	defer procUnhookWindowsHook.Call(hook)

	log.Println("capture app is running; keyboard hook enabled for double Ctrl+C")
	runMessageLoop()
}

func runMessageLoop() {
	var message msg
	for {
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		switch int32(ret) {
		case -1:
			log.Println("message loop error")
			return
		case 0:
			return
		default:
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
			procDispatchMessage.Call(uintptr(unsafe.Pointer(&message)))
		}
	}
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
					go handleConfirmedCapture()
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

func isCtrlKey(vk uint32) bool {
	return vk == vkControl || vk == vkLControl || vk == vkRControl
}

func handleConfirmedCapture() {
	time.Sleep(300 * time.Millisecond)

	text, err := clipboard.ReadAll()
	if err != nil {
		log.Printf("read clipboard: %v", err)
		notify("抓取失败", "无法读取剪贴板内容")
		return
	}
	log.Printf("clipboard read after double Ctrl+C, chars=%d", len([]rune(text)))

	result, err := captureServiceInstance.CaptureClipboard(context.Background(), text)
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
	tryAutoAnalyze(result.ID)
}

func tryAutoAnalyze(captureID int64) {
	configPath := getenv("AIHELPER_CONFIG_PATH", "config/app.json")
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Printf("load config for auto analyze: %v", err)
		return
	}
	if !cfg.Capture.AutoAnalyze {
		return
	}

	repo := repository.NewSQLiteSolutionRepository(dbInstance)
	solutionSvc := service.NewSolutionService(repo)
	analysisSvc := service.NewCaptureAnalysisService(captureServiceInstance, solutionSvc, extractor.New(cfg.AI))

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
		"，字符数: " + itoa(result.CharCount) +
		"，行数: " + itoa(result.LineCount)
}

func formatDuplicateMessage(result *capture.Result) string {
	return "内容之前已抓取过，类型: " + result.ContentType +
		"，字符数: " + itoa(result.CharCount)
}

func notify(title, message string) {
	if err := beeep.Notify(title, message, ""); err != nil {
		log.Printf("notify: %v", err)
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func itoa(v int) string {
	return strconv.Itoa(v)
}
