package localapi

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var extensionIDPattern = regexp.MustCompile(`^[a-p]{32}$`)

type Discovery struct {
	Address          string   `json:"address"`
	HostToken        string   `json:"hostToken"`
	AgentToken       string   `json:"agentToken"`
	ExtensionOrigins []string `json:"extensionOrigins"`
}

func NewDiscovery(address string, extensionOrigins []string) (Discovery, error) {
	if !isLoopbackAddress(address) {
		return Discovery{}, errors.New("discovery address must be loopback")
	}
	var host, agent [32]byte
	if _, err := rand.Read(host[:]); err != nil {
		return Discovery{}, errors.New("could not generate host credential")
	}
	if _, err := rand.Read(agent[:]); err != nil {
		return Discovery{}, errors.New("could not generate agent credential")
	}
	if subtle.ConstantTimeCompare(host[:], agent[:]) == 1 {
		if _, err := rand.Read(agent[:]); err != nil {
			return Discovery{}, errors.New("could not generate agent credential")
		}
	}
	origins := make([]string, 0, len(extensionOrigins))
	for _, origin := range extensionOrigins {
		canonical, err := normalizeChromeExtensionOrigin(origin)
		if err != nil {
			return Discovery{}, err
		}
		origins = append(origins, canonical)
	}
	return Discovery{Address: address, HostToken: hex.EncodeToString(host[:]), AgentToken: hex.EncodeToString(agent[:]), ExtensionOrigins: origins}, nil
}

func DiscoveryPath() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", errors.New("user configuration directory is unavailable")
	}
	return filepath.Join(root, "LegacyLens", "discovery.json"), nil
}

func EnsurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return errors.New("could not create the per-user configuration directory")
	}
	if err := secureUserPath(path, true); err != nil {
		return errors.New("could not protect the per-user configuration directory")
	}
	return nil
}

func WriteDiscovery(path string, discovery Discovery) error {
	if !isLoopbackAddress(discovery.Address) || len(discovery.HostToken) < 32 || len(discovery.AgentToken) < 32 {
		return errors.New("discovery configuration is invalid")
	}
	directory := filepath.Dir(path)
	if err := EnsurePrivateDirectory(directory); err != nil {
		return err
	}
	data, err := json.Marshal(discovery)
	if err != nil {
		return errors.New("discovery configuration could not be encoded")
	}
	temporary, err := os.CreateTemp(directory, ".discovery-*.tmp")
	if err != nil {
		return errors.New("could not create the discovery configuration")
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0600); err != nil {
		_ = temporary.Close()
		return errors.New("could not protect the discovery configuration")
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return errors.New("could not write the discovery configuration")
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return errors.New("could not flush the discovery configuration")
	}
	if err := temporary.Close(); err != nil {
		return errors.New("could not close the discovery configuration")
	}
	if err := secureUserPath(temporaryName, false); err != nil {
		return errors.New("could not protect the discovery configuration")
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return errors.New("could not publish the discovery configuration")
	}
	if err := secureUserPath(path, false); err != nil {
		return errors.New("could not protect the discovery configuration")
	}
	return nil
}

func ReadDiscovery(path string) (Discovery, error) {
	if err := secureUserPath(filepath.Dir(path), true); err != nil {
		return Discovery{}, errors.New("discovery configuration is unavailable")
	}
	if err := secureUserPath(path, false); err != nil {
		return Discovery{}, errors.New("discovery configuration is unavailable")
	}
	file, err := os.Open(path)
	if err != nil {
		return Discovery{}, errors.New("discovery configuration is unavailable")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 16*1024+1))
	if err != nil || len(data) > 16*1024 {
		return Discovery{}, errors.New("discovery configuration is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var discovery Discovery
	if err := decoder.Decode(&discovery); err != nil || decoder.Decode(new(any)) != io.EOF || !isLoopbackAddress(discovery.Address) || len(discovery.HostToken) < 32 || len(discovery.AgentToken) < 32 || discovery.HostToken == discovery.AgentToken {
		return Discovery{}, errors.New("discovery configuration is invalid")
	}
	for _, origin := range discovery.ExtensionOrigins {
		if _, err := normalizeChromeExtensionOrigin(origin); err != nil {
			return Discovery{}, errors.New("discovery configuration is invalid")
		}
	}
	return discovery, nil
}

func isLoopbackAddress(address string) bool {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port == "" {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func extensionOrigin(id string) (string, error) {
	id = strings.TrimSpace(id)
	if !extensionIDPattern.MatchString(id) || strings.ContainsAny(id, "/:\r\n") {
		return "", errors.New("extension id is invalid")
	}
	return "chrome-extension://" + id, nil
}

func ChromeExtensionOrigin(id string) (string, error) { return extensionOrigin(id) }

func normalizeChromeExtensionOrigin(value string) (string, error) {
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "chrome-extension" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("extension origin is invalid")
	}
	return extensionOrigin(u.Host)
}
