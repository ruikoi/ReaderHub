package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"readerhub/internal/document"
)

type Client struct {
	HubURL   string `json:"hub_url"`
	Token    string `json:"token"`
	DeviceID string `json:"device_id"`
	Machine  string `json:"machine"`
}

func DefaultClientPath() string {
	if p := os.Getenv("READERHUB_CLI_CONFIG"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "readerhub-cli", "config.json")
}

func ReadClient(path string) (Client, error) {
	var c Client
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		c.DeviceID = document.NewID()
		c.Machine, _ = os.Hostname()
		return c, nil
	}
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(b, &c)
	return c, err
}

func WriteClient(path string, c Client) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWrite(path, b, 0600)
}

func HubToken(dataDir string) (string, error) {
	path := filepath.Join(dataDir, "access-token")
	b, err := os.ReadFile(path)
	if err == nil {
		if len(b) != 64 {
			return "", errors.New("invalid access-token file")
		}
		return string(b), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw[:])
	if err := AtomicWrite(path, []byte(token), 0600); err != nil {
		return "", err
	}
	return token, nil
}

func AtomicWrite(path string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".write-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
