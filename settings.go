package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	modeAnon    = "anonymous"
	modeAccount = "account"
	modeLast    = "last" // quick upload follows whichever mode was used last
)

type Settings struct {
	AccountID           string `json:"account_id"`
	QuickMode           string `json:"quick_mode"`
	LastMode            string `json:"last_mode"`
	DefaultLocationID   string `json:"default_location_id"`
	DefaultLocationName string `json:"default_location_name"`
	QuickAsksOptions    bool   `json:"quick_upload_asks_options"`
	AutoUpdate          bool   `json:"auto_update"`

	dir      string
	imported string // path of a v1 settings file that was imported on this run
}

func defaultSettings(dir string) *Settings {
	return &Settings{QuickMode: modeLast, LastMode: modeAnon, dir: dir}
}

func configDir() (string, error) {
	if d := os.Getenv("BUZZHEAVIER_CONFIG_DIR"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "BuzzheavierClient"), nil
}

func (s *Settings) path() string    { return filepath.Join(s.dir, "settings.json") }
func (s *Settings) logPath() string { return filepath.Join(s.dir, "uploads.log") }

func loadSettings() (*Settings, error) {
	dir, err := configDir()
	if err != nil {
		return defaultSettings(""), err
	}
	s := defaultSettings(dir)
	b, err := os.ReadFile(s.path())
	if errors.Is(err, fs.ErrNotExist) {
		s.importV1()
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, s); err != nil {
		return defaultSettings(dir), fmt.Errorf("settings file is damaged, using defaults: %w", err)
	}
	s.normalize()
	return s, nil
}

func (s *Settings) normalize() {
	s.AccountID = strings.TrimSpace(s.AccountID)
	switch s.QuickMode {
	case modeAnon, modeAccount, modeLast:
	default:
		s.QuickMode = modeLast
	}
	if s.LastMode != modeAccount {
		s.LastMode = modeAnon
	}
}

func (s *Settings) Save() error {
	if s.dir == "" {
		return errors.New("no settings folder available")
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path() + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path())
}

// QuickUploadMode is the mode used for drag-and-drop / pasted-path uploads.
func (s *Settings) QuickUploadMode() string {
	m := s.QuickMode
	if m == modeLast {
		m = s.LastMode
	}
	if m == modeAccount && s.AccountID == "" {
		return modeAnon
	}
	return m
}

// importV1 picks up buzz_settings.ini written by the old Windows batch client,
// looking next to the executable and in the current folder.
func (s *Settings) importV1() {
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "buzz_settings.ini"))
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "buzz_settings.ini"))
	}
	for _, p := range candidates {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			k, v, ok := strings.Cut(sc.Text(), "=")
			if !ok {
				continue
			}
			v = strings.TrimSpace(v)
			switch strings.ToLower(strings.TrimSpace(k)) {
			case "account":
				s.AccountID = v
			case "last_mode":
				if v == "token" || v == modeAccount {
					s.LastMode = modeAccount
				}
			}
		}
		f.Close()
		if s.AccountID != "" {
			s.normalize()
			if s.Save() == nil {
				s.imported = p
			}
			return
		}
	}
}

// ---- Upload history ----

func (s *Settings) appendLog(name, link, mode string) error {
	if s.dir == "" {
		return errors.New("no settings folder available")
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(s.logPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "[%s] - %s - %s - %s\n", time.Now().Format("2006-01-02 15:04:05"), name, link, mode)
	return err
}

func (s *Settings) readLog(last int) ([]string, error) {
	b, err := os.ReadFile(s.logPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, l := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) > last {
		lines = lines[len(lines)-last:]
	}
	return lines, nil
}
