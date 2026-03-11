package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/viper"
)

type storedConfig struct {
	Cookie            string  `json:"cookie"`
	ReportIntervalSec int     `json:"report_interval_sec,omitempty"`
	SpeedMultiplier   float64 `json:"speed_multiplier,omitempty"`
	Concurrency       int     `json:"concurrency,omitempty"`
	FFProbePath       string  `json:"ffprobe_path,omitempty"`
}

type RuntimeConfig struct {
	ReportIntervalSec int
	SpeedMultiplier   float64
	Concurrency       int
	FFProbePath       string
}

type AppConfig struct {
	Cookie string
	RuntimeConfig
}

const (
	defaultReportIntervalSec = 20
	defaultSpeedMultiplier   = 2.0
	defaultConcurrency       = 3
)

func ResolveCookie(cookieArg string) (string, error) {
	cookieArg = strings.TrimSpace(cookieArg)
	if cookieArg != "" {
		return cookieArg, nil
	}

	if cfg, err := load(); err == nil && strings.TrimSpace(cfg.Cookie) != "" {
		return strings.TrimSpace(cfg.Cookie), nil
	}

	fmt.Print("Input browser cookie: ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && len(strings.TrimSpace(line)) == 0 {
		return "", fmt.Errorf("cannot read cookie: %w", err)
	}
	cookie := strings.TrimSpace(line)
	if cookie == "" {
		return "", fmt.Errorf("cookie cannot be empty")
	}
	return cookie, nil
}

func SaveCookie(cookie string) error {
	cookie = strings.TrimSpace(cookie)
	if cookie == "" {
		return fmt.Errorf("cookie cannot be empty")
	}

	cfg, err := load()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	cfg.Cookie = cookie
	return save(cfg)
}

func LoadRuntimeConfig() (RuntimeConfig, error) {
	cfg, err := load()
	if err != nil {
		if os.IsNotExist(err) {
			return RuntimeConfig{
				ReportIntervalSec: defaultReportIntervalSec,
				SpeedMultiplier:   defaultSpeedMultiplier,
				Concurrency:       defaultConcurrency,
				FFProbePath:       "",
			}, nil
		}
		return RuntimeConfig{}, err
	}

	rc := RuntimeConfig{
		ReportIntervalSec: cfg.ReportIntervalSec,
		SpeedMultiplier:   cfg.SpeedMultiplier,
		Concurrency:       cfg.Concurrency,
		FFProbePath:       strings.TrimSpace(cfg.FFProbePath),
	}
	if rc.ReportIntervalSec <= 0 {
		rc.ReportIntervalSec = defaultReportIntervalSec
	}
	if rc.SpeedMultiplier <= 0 {
		rc.SpeedMultiplier = defaultSpeedMultiplier
	}
	if rc.Concurrency <= 0 {
		rc.Concurrency = defaultConcurrency
	}
	return rc, nil
}

func SaveRuntimeConfig(runtime RuntimeConfig) error {
	if runtime.ReportIntervalSec <= 0 {
		return fmt.Errorf("report interval must be > 0")
	}
	if runtime.SpeedMultiplier <= 0 {
		return fmt.Errorf("speed multiplier must be > 0")
	}
	if runtime.Concurrency <= 0 {
		return fmt.Errorf("concurrency must be > 0")
	}

	cfg, err := load()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	cfg.ReportIntervalSec = runtime.ReportIntervalSec
	cfg.SpeedMultiplier = runtime.SpeedMultiplier
	cfg.Concurrency = runtime.Concurrency
	cfg.FFProbePath = strings.TrimSpace(runtime.FFProbePath)
	return save(cfg)
}

func LoadAppConfig() (AppConfig, string, error) {
	cfg, path, err := loadWithPath()
	if err != nil {
		if os.IsNotExist(err) {
			active, pErr := ActiveConfigPath()
			if pErr != nil {
				return AppConfig{}, "", pErr
			}
			return AppConfig{
				Cookie: "",
				RuntimeConfig: RuntimeConfig{
					ReportIntervalSec: defaultReportIntervalSec,
					SpeedMultiplier:   defaultSpeedMultiplier,
					Concurrency:       defaultConcurrency,
					FFProbePath:       "",
				},
			}, active, nil
		}
		return AppConfig{}, "", err
	}

	rc := RuntimeConfig{
		ReportIntervalSec: cfg.ReportIntervalSec,
		SpeedMultiplier:   cfg.SpeedMultiplier,
		Concurrency:       cfg.Concurrency,
		FFProbePath:       strings.TrimSpace(cfg.FFProbePath),
	}
	if rc.ReportIntervalSec <= 0 {
		rc.ReportIntervalSec = defaultReportIntervalSec
	}
	if rc.SpeedMultiplier <= 0 {
		rc.SpeedMultiplier = defaultSpeedMultiplier
	}
	if rc.Concurrency <= 0 {
		rc.Concurrency = defaultConcurrency
	}

	return AppConfig{Cookie: strings.TrimSpace(cfg.Cookie), RuntimeConfig: rc}, path, nil
}

func ActiveConfigPath() (string, error) {
	return writableConfigPath()
}

func load() (storedConfig, error) {
	cfg, _, err := loadWithPath()
	return cfg, err
}

func loadWithPath() (storedConfig, string, error) {
	paths, err := configPaths()
	if err != nil {
		return storedConfig{}, "", err
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return storedConfig{}, "", err
		}

		v := viper.New()
		v.SetConfigFile(path)
		v.SetConfigType("json")
		if err := v.ReadInConfig(); err != nil {
			return storedConfig{}, "", fmt.Errorf("read config: %w", err)
		}

		cfg := storedConfig{
			Cookie:            strings.TrimSpace(v.GetString("cookie")),
			ReportIntervalSec: v.GetInt("report_interval_sec"),
			SpeedMultiplier:   v.GetFloat64("speed_multiplier"),
			Concurrency:       v.GetInt("concurrency"),
			FFProbePath:       strings.TrimSpace(v.GetString("ffprobe_path")),
		}
		return cfg, path, nil
	}

	return storedConfig{}, "", os.ErrNotExist
}

func save(cfg storedConfig) error {
	path, err := writableConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	v := viper.New()
	v.Set("cookie", cfg.Cookie)
	v.Set("report_interval_sec", cfg.ReportIntervalSec)
	v.Set("speed_multiplier", cfg.SpeedMultiplier)
	v.Set("concurrency", cfg.Concurrency)
	v.Set("ffprobe_path", cfg.FFProbePath)
	v.SetConfigFile(path)
	v.SetConfigType("json")

	if _, err := os.Stat(path); err == nil {
		if err := v.WriteConfigAs(path); err != nil {
			return fmt.Errorf("write config: %w", err)
		}
	} else if os.IsNotExist(err) {
		if err := v.SafeWriteConfigAs(path); err != nil {
			return fmt.Errorf("write config: %w", err)
		}
	} else {
		return fmt.Errorf("stat config: %w", err)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o600); err != nil {
			return fmt.Errorf("chmod config: %w", err)
		}
	}
	return nil
}

func configPaths() ([]string, error) {
	programPath, err := programConfigPath()
	if err != nil {
		return nil, err
	}
	standardPath, err := standardConfigPath()
	if err != nil {
		return nil, err
	}

	if programPath == standardPath {
		return []string{programPath}, nil
	}
	return []string{programPath, standardPath}, nil
}

func writableConfigPath() (string, error) {
	programPath, err := programConfigPath()
	if err != nil {
		return "", err
	}
	if canWriteConfigPath(programPath) {
		return programPath, nil
	}

	return standardConfigPath()
}

func canWriteConfigPath(path string) bool {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false
	}
	probe := filepath.Join(dir, ".write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return false
	}
	_ = os.Remove(probe)
	return true
}

func programConfigPath() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable: %w", err)
	}
	exeDir := filepath.Dir(exePath)
	return filepath.Join(exeDir, ".config", "config.json"), nil
}

func standardConfigPath() (string, error) {
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); strings.TrimSpace(appData) != "" {
			return filepath.Join(appData, "auto-uooc-go", "config.json"), nil
		}
		userConfigDir, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("resolve user config dir: %w", err)
		}
		return filepath.Join(userConfigDir, "auto-uooc-go", "config.json"), nil
	}

	if xdg := os.Getenv("XDG_CONFIG_HOME"); strings.TrimSpace(xdg) != "" {
		return filepath.Join(xdg, "auto-uooc-go", "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "auto-uooc-go", "config.json"), nil
}
