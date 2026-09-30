package localapi

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDiscoveryRoundTripKeepsSeparateCredentialsAndPrivateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "discovery.json")
	discovery, err := NewDiscovery("127.0.0.1:43127", []string{"chrome-extension://abcdefghijklmnopabcdefghijklmnop/"})
	if err != nil {
		t.Fatal(err)
	}
	if discovery.HostToken == discovery.AgentToken {
		t.Fatal("host and agent credentials must be independent")
	}
	if err := WriteDiscovery(path, discovery); err != nil {
		t.Fatal(err)
	}
	loaded, err := ReadDiscovery(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.HostToken != discovery.HostToken || loaded.AgentToken != discovery.AgentToken || len(loaded.ExtensionOrigins) != 1 || loaded.ExtensionOrigins[0] != "chrome-extension://abcdefghijklmnopabcdefghijklmnop" {
		t.Fatalf("discovery round trip = %+v", loaded)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("discovery permissions = %04o, want 0600", info.Mode().Perm())
		}
	}
}
