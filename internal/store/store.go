package store

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"irroute/internal/model"
)

type Paths struct {
	Root       string
	ConfigDir  string
	StateDir   string
	DataDir    string
	BackupDir  string
	RunDir     string
	ConfigFile string
	StateFile  string
	IranFile   string
}

func NewPaths(root string) Paths {
	if root == "" {
		root = "/"
	}
	join := func(path string) string {
		if root == "/" {
			return path
		}
		return filepath.Join(root, strings.TrimPrefix(path, "/"))
	}
	configDir := join("/etc/irroute")
	stateDir := join("/var/lib/irroute")
	return Paths{
		Root:       root,
		ConfigDir:  configDir,
		StateDir:   stateDir,
		DataDir:    filepath.Join(stateDir, "data"),
		BackupDir:  filepath.Join(stateDir, "backups"),
		RunDir:     join("/run/irroute"),
		ConfigFile: filepath.Join(configDir, "config.json"),
		StateFile:  filepath.Join(stateDir, "state.json"),
		IranFile:   filepath.Join(stateDir, "data", "iran-current.cidr"),
	}
}

type Store struct {
	Paths Paths
}

type Lock struct {
	file *os.File
}

func New(paths Paths) *Store {
	return &Store{Paths: paths}
}

func (s *Store) AcquireLock() (*Lock, error) {
	if err := s.EnsureDirectories(); err != nil {
		return nil, err
	}
	path := filepath.Join(s.Paths.RunDir, "irroute.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, errors.New("another irroute operation is already running")
	}
	return &Lock{file: file}, nil
}

func (lock *Lock) Release() {
	if lock == nil || lock.file == nil {
		return
	}
	_ = syscall.Flock(int(lock.file.Fd()), syscall.LOCK_UN)
	_ = lock.file.Close()
}

func (s *Store) EnsureDirectories() error {
	for _, item := range []struct {
		path string
		mode os.FileMode
	}{
		{s.Paths.ConfigDir, 0o700},
		{s.Paths.StateDir, 0o700},
		{s.Paths.DataDir, 0o700},
		{s.Paths.BackupDir, 0o700},
		{s.Paths.RunDir, 0o700},
	} {
		if err := os.MkdirAll(item.path, item.mode); err != nil {
			return fmt.Errorf("create %s: %w", item.path, err)
		}
		if err := os.Chmod(item.path, item.mode); err != nil {
			return fmt.Errorf("set permissions on %s: %w", item.path, err)
		}
	}
	return nil
}

func (s *Store) LoadConfig() (model.Config, error) {
	var cfg model.Config
	if err := readJSON(s.Paths.ConfigFile, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (s *Store) SaveConfig(cfg model.Config) error {
	if err := s.EnsureDirectories(); err != nil {
		return err
	}
	return writeJSONAtomic(s.Paths.ConfigFile, cfg, 0o600)
}

func (s *Store) LoadState() (model.State, error) {
	var state model.State
	err := readJSON(s.Paths.StateFile, &state)
	if errors.Is(err, os.ErrNotExist) {
		return model.DefaultState(), nil
	}
	return state, err
}

func (s *Store) SaveState(state model.State) error {
	if err := s.EnsureDirectories(); err != nil {
		return err
	}
	return writeJSONAtomic(s.Paths.StateFile, state, 0o600)
}

func (s *Store) LoadIranCIDRs() ([]string, error) {
	file, err := os.Open(s.Paths.IranFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	seen := make(map[string]struct{})
	var result []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.SplitN(scanner.Text(), "#", 2)[0])
		if line == "" {
			continue
		}
		if !strings.Contains(line, "/") {
			line += "/32"
		}
		prefix, err := netip.ParsePrefix(line)
		if err != nil || !prefix.Addr().Is4() {
			return nil, fmt.Errorf("invalid IPv4 CIDR %q in %s", line, s.Paths.IranFile)
		}
		value := prefix.Masked().String()
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", s.Paths.IranFile, err)
	}
	sort.Strings(result)
	if len(result) == 0 {
		return nil, errors.New("Iran CIDR data is empty")
	}
	return result, nil
}

func (s *Store) ImportIranCIDRs(source string) (int, string, error) {
	file, err := os.Open(source)
	if err != nil {
		return 0, "", err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, (16<<20)+1))
	if err != nil {
		return 0, "", err
	}
	if len(data) > 16<<20 {
		return 0, "", errors.New("CIDR data exceeds the 16 MiB safety limit")
	}
	lines, err := normalizeCIDRData(string(data))
	if err != nil {
		return 0, "", err
	}
	if err := s.EnsureDirectories(); err != nil {
		return 0, "", err
	}
	payload := []byte(strings.Join(lines, "\n") + "\n")
	if err := writeFileAtomic(s.Paths.IranFile, payload, 0o600); err != nil {
		return 0, "", err
	}
	hash := sha256.Sum256(payload)
	return len(lines), hex.EncodeToString(hash[:]), nil
}

func (s *Store) WriteSnapshot(reason string, cfg model.Config, state model.State, iran []string) (string, error) {
	if err := s.EnsureDirectories(); err != nil {
		return "", err
	}
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	path := filepath.Join(s.Paths.BackupDir, stamp+".json")
	snapshot := model.Snapshot{
		SchemaVersion: model.SchemaVersion,
		CreatedAt:     time.Now().UTC(),
		Reason:        reason,
		Config:        cfg,
		State:         state,
		IranCIDRs:     append([]string(nil), iran...),
	}
	if err := writeJSONAtomic(path, snapshot, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (s *Store) LoadSnapshot(path string) (model.Snapshot, error) {
	var snapshot model.Snapshot
	if path == "" || path == "latest" {
		entries, err := filepath.Glob(filepath.Join(s.Paths.BackupDir, "*.json"))
		if err != nil || len(entries) == 0 {
			return snapshot, errors.New("no rollback snapshot is available")
		}
		sort.Strings(entries)
		path = entries[len(entries)-1]
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.Paths.BackupDir, path)
	}
	if err := readJSON(path, &snapshot); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

func (s *Store) RestoreData(cidrs []string) error {
	if len(cidrs) == 0 {
		return errors.New("snapshot contains no Iran CIDR data")
	}
	if err := s.EnsureDirectories(); err != nil {
		return err
	}
	payload := []byte(strings.Join(cidrs, "\n") + "\n")
	return writeFileAtomic(s.Paths.IranFile, payload, 0o600)
}

func normalizeCIDRData(data string) ([]string, error) {
	seen := make(map[string]struct{})
	var result []string
	for lineNumber, raw := range strings.Split(data, "\n") {
		line := strings.TrimSpace(strings.SplitN(raw, "#", 2)[0])
		if line == "" {
			continue
		}
		if !strings.Contains(line, "/") {
			line += "/32"
		}
		prefix, err := netip.ParsePrefix(line)
		if err != nil || !prefix.Addr().Is4() {
			return nil, fmt.Errorf("line %d contains an invalid IPv4 CIDR: %s", lineNumber+1, line)
		}
		value := prefix.Masked().String()
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil, errors.New("CIDR source contains no usable IPv4 networks")
	}
	sort.Strings(result)
	return result, nil
}

func readJSON(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func writeJSONAtomic(path string, value any, mode os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFileAtomic(path, data, mode)
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".irroute-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, path)
}
