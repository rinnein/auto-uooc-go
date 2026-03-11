package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"auto-uooc-go/internal/api"
	"auto-uooc-go/internal/config"
	"auto-uooc-go/internal/runner"
)

const (
	defaultRequestTimeout = 20 * time.Second
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "auth":
		if err := runAuth(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "auth failed: %v\n", err)
			os.Exit(1)
		}
	case "config":
		if err := runConfig(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "config failed: %v\n", err)
			os.Exit(1)
		}
	case "run":
		if err := runCourse(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "run failed: %v\n", err)
			os.Exit(1)
		}
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  auto-uooc-go auth [--cookie \"k=v; ...\"]")
	fmt.Println("  auto-uooc-go config [--show] [--cookie \"k=v; ...\"] [--report-interval 20] [--speed-multiplier 2] [--concurrency 3] [--ffprobe-path /usr/bin/ffprobe] [--clear-ffprobe-path]")
	fmt.Println("  auto-uooc-go run --cid <course_id> [--cookie \"k=v; ...\"] [--dry-run] [--report-interval 20] [--speed-multiplier 2] [--concurrency 3] [--ffprobe-path /usr/bin/ffprobe]")
}

func runConfig(args []string) error {
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	show := fs.Bool("show", false, "Show current config")
	cookie := fs.String("cookie", "", "Set cookie")
	reportIntervalSec := fs.Int("report-interval", 0, "Set report interval in seconds")
	speedMultiplier := fs.Float64("speed-multiplier", 0, "Set speed multiplier")
	concurrency := fs.Int("concurrency", 0, "Set max concurrent video tasks")
	ffprobePath := fs.String("ffprobe-path", "", "Set ffprobe binary path")
	clearFFProbePath := fs.Bool("clear-ffprobe-path", false, "Clear ffprobe_path in config")
	if err := fs.Parse(args); err != nil {
		return err
	}

	changed := false
	if *cookie != "" {
		if err := config.SaveCookie(*cookie); err != nil {
			return err
		}
		changed = true
	}

	if *reportIntervalSec > 0 || *speedMultiplier > 0 || *concurrency > 0 || *ffprobePath != "" || *clearFFProbePath {
		runtimeCfg, err := config.LoadRuntimeConfig()
		if err != nil {
			return err
		}
		if *reportIntervalSec > 0 {
			runtimeCfg.ReportIntervalSec = *reportIntervalSec
		}
		if *speedMultiplier > 0 {
			runtimeCfg.SpeedMultiplier = *speedMultiplier
		}
		if *concurrency > 0 {
			runtimeCfg.Concurrency = *concurrency
		}
		if *ffprobePath != "" {
			runtimeCfg.FFProbePath = *ffprobePath
		}
		if *clearFFProbePath {
			runtimeCfg.FFProbePath = ""
		}
		if err := config.SaveRuntimeConfig(runtimeCfg); err != nil {
			return err
		}
		changed = true
	}

	if *show {
		appCfg, loadedPath, err := config.LoadAppConfig()
		if err != nil {
			return err
		}
		fmt.Printf("config_path=%s\n", loadedPath)
		fmt.Printf("cookie_set=%v\n", appCfg.Cookie != "")
		fmt.Printf("report_interval_sec=%d\n", appCfg.ReportIntervalSec)
		fmt.Printf("speed_multiplier=%.2f\n", appCfg.SpeedMultiplier)
		fmt.Printf("concurrency=%d\n", appCfg.Concurrency)
		fmt.Printf("ffprobe_path=%s\n", appCfg.FFProbePath)
		return nil
	}

	if !changed {
		return fmt.Errorf("no config option provided, use --show or set at least one option")
	}

	path, err := config.ActiveConfigPath()
	if err != nil {
		return err
	}
	fmt.Printf("config updated: %s\n", path)
	return nil
}

func runAuth(args []string) error {
	fs := flag.NewFlagSet("auth", flag.ContinueOnError)
	cookieFlag := fs.String("cookie", "", "Cookie string copied from browser")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cookie, err := config.ResolveCookie(*cookieFlag)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultRequestTimeout)
	defer cancel()

	client := api.NewClient(cookie)
	user, err := client.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("cookie is invalid or expired: %w", err)
	}

	if err := config.SaveCookie(cookie); err != nil {
		return err
	}

	fmt.Printf("login ok, uid=%d, nick=%s\n", user.ID, user.Nick)
	fmt.Println("cookie saved")
	return nil
}

func runCourse(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	cid := fs.Int64("cid", 0, "course id")
	cookieFlag := fs.String("cookie", "", "Cookie string copied from browser")
	dryRun := fs.Bool("dry-run", false, "Print progress plan without calling report API")
	reportIntervalSec := fs.Int("report-interval", 0, "Report interval in seconds, default from config (20)")
	speedMultiplier := fs.Float64("speed-multiplier", 0, "Progress speed multiplier, progress step = interval * multiplier (default 2)")
	concurrency := fs.Int("concurrency", 0, "Max concurrent video tasks, default from config (3)")
	ffprobePath := fs.String("ffprobe-path", "", "ffprobe binary path; empty uses config/env/cwd fallback")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *cid <= 0 {
		return fmt.Errorf("--cid is required")
	}

	cookie, err := config.ResolveCookie(*cookieFlag)
	if err != nil {
		return err
	}
	if *cookieFlag != "" {
		if err := config.SaveCookie(cookie); err != nil {
			return err
		}
	}

	runtimeCfg, err := config.LoadRuntimeConfig()
	if err != nil {
		return err
	}
	configChanged := false
	if *reportIntervalSec > 0 {
		runtimeCfg.ReportIntervalSec = *reportIntervalSec
		configChanged = true
	}
	if *speedMultiplier > 0 {
		runtimeCfg.SpeedMultiplier = *speedMultiplier
		configChanged = true
	}
	if *concurrency > 0 {
		runtimeCfg.Concurrency = *concurrency
		configChanged = true
	}
	if *ffprobePath != "" {
		runtimeCfg.FFProbePath = *ffprobePath
		configChanged = true
	}
	if configChanged {
		if err := config.SaveRuntimeConfig(runtimeCfg); err != nil {
			return err
		}
	}

	client := api.NewClient(cookie)
	r := runner.New(client, runner.Options{
		DryRun:          *dryRun,
		Concurrency:     runtimeCfg.Concurrency,
		ReportInterval:  time.Duration(runtimeCfg.ReportIntervalSec) * time.Second,
		SpeedMultiplier: runtimeCfg.SpeedMultiplier,
		FFProbePath:     runtimeCfg.FFProbePath,
	})
	return r.RunCourse(context.Background(), *cid)
}
