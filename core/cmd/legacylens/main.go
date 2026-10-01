package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"legacylens/core/internal/adapters/filesystem"
	"legacylens/core/internal/adapters/intellij"
	"legacylens/core/internal/adapters/localapi"
	"legacylens/core/internal/adapters/sqlite"
	"legacylens/core/internal/adapters/static/xhtml"
	"legacylens/core/internal/application"
	"legacylens/core/internal/domain"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: legacylens serve|status|project register|project list|index|inspect --trace")
	}
	switch args[0] {
	case "serve":
		return serve()
	case "status":
		return status()
	case "project":
		if len(args) < 2 {
			return errors.New("project requires register or list")
		}
		switch args[1] {
		case "register":
			return registerProject(args[2:])
		case "list":
			return listProjects()
		default:
			return errors.New("project requires register or list")
		}
	case "index":
		return indexProject(args[1:])
	case "inspect":
		return inspectTrace(args[1:])
	default:
		return errors.New("unknown command; use serve, status, project register, project list, index, or inspect --trace")
	}
}

func configDirectory() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", errors.New("user configuration directory is unavailable")
	}
	directory := filepath.Join(root, "LegacyLens")
	if err := localapi.EnsurePrivateDirectory(directory); err != nil {
		return "", err
	}
	return directory, nil
}

func openStore() (*sqlite.Store, error) {
	directory, err := configDirectory()
	if err != nil {
		return nil, err
	}
	store, err := sqlite.Open(filepath.Join(directory, "legacylens.sqlite"))
	if err != nil {
		return nil, errors.New("local project database could not be opened")
	}
	return store, nil
}

func runtimeServices(store *sqlite.Store) localapi.Services {
	projects := application.NewProjectService(store)
	indexer := application.NewIndexer(store, store, filesystem.NewSource(), []application.Analyzer{xhtml.NewAnalyzer()})
	captures := application.NewCaptureService(store, application.CaptureConfig{})
	locations := application.NewLocationService(store, intellij.NewEditor(os.Getenv("LEGACYLENS_INTELLIJ_LAUNCHER"), nil))
	presence := application.NewAgentPresence(time.Now, 15*time.Second, 128)
	return localapi.Services{Projects: projects, Indexer: indexer, Captures: captures, Presence: presence, Investigations: application.NewInvestigationService(store).WithAgentPresence(presence), Locations: locations}
}

func serve() error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return errors.New("loopback listener could not start")
	}
	defer listener.Close()
	var origins []string
	if extensionID := strings.TrimSpace(os.Getenv("LEGACYLENS_EXTENSION_ID")); extensionID != "" {
		origin, err := localapi.ChromeExtensionOrigin(extensionID)
		if err != nil {
			return err
		}
		origins = append(origins, origin)
	}
	discovery, err := localapi.NewDiscovery(listener.Addr().String(), origins)
	if err != nil {
		return err
	}
	discoveryPath, err := localapi.DiscoveryPath()
	if err != nil {
		return err
	}
	if err := localapi.WriteDiscovery(discoveryPath, discovery); err != nil {
		return err
	}
	defer os.Remove(discoveryPath)
	handler := localapi.NewServer(runtimeServices(store), localapi.AuthConfig{HostToken: discovery.HostToken, AgentToken: discovery.AgentToken, ExtensionOrigins: discovery.ExtensionOrigins})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	_, _ = fmt.Fprintln(os.Stdout, "LegacyLens local core is running")
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return errors.New("local core stopped unexpectedly")
	}
}

func status() error {
	path, err := localapi.DiscoveryPath()
	if err != nil {
		return err
	}
	discovery, err := localapi.ReadDiscovery(path)
	if err != nil {
		return errors.New("LegacyLens local core is not running")
	}
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: nil}}
	response, err := client.Get("http://" + discovery.Address + "/v1/health")
	if err != nil {
		return errors.New("LegacyLens local core is not responding")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New("LegacyLens local core is not healthy")
	}
	_, _ = fmt.Fprintln(os.Stdout, "LegacyLens local core is healthy")
	return nil
}

func registerProject(args []string) error {
	flags := flag.NewFlagSet("project register", flag.ContinueOnError)
	root := flags.String("root", "", "project root directory")
	name := flags.String("name", "", "project display name")
	includes := flags.String("includes", "", "comma-separated include globs")
	excludes := flags.String("excludes", "", "comma-separated exclude globs")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*root) == "" {
		return errors.New("project register requires --root")
	}
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()
	project, err := application.NewProjectService(store).RegisterProject(context.Background(), application.ProjectConfig{Root: *root, Name: *name, Includes: splitList(*includes), Excludes: splitList(*excludes)})
	if err != nil {
		return errors.New("project could not be registered")
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"projectId": project.ID, "status": "registered"})
}

func listProjects() error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()
	projects, err := application.NewProjectService(store).ListProjects(context.Background())
	if err != nil {
		return errors.New("projects could not be listed")
	}
	return json.NewEncoder(os.Stdout).Encode(projects)
}

func indexProject(args []string) error {
	flags := flag.NewFlagSet("index", flag.ContinueOnError)
	projectID := flags.String("project", "", "registered project id")
	var paths stringList
	flags.Var(&paths, "path", "relative project file (repeatable)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *projectID == "" {
		return errors.New("index requires --project")
	}
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()
	services := runtimeServices(store)
	result, err := services.Indexer.Index(context.Background(), application.IndexRequest{ProjectID: domain.ID(*projectID), Paths: paths})
	if err != nil {
		return errors.New("project indexing failed")
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"projectId": result.ProjectID, "revisionId": result.RevisionID, "artifacts": len(result.Artifacts), "symbols": len(result.Symbols), "relations": len(result.Relations), "diagnostics": result.Diagnostics})
}

func inspectTrace(args []string) error {
	flags := flag.NewFlagSet("inspect", flag.ContinueOnError)
	projectID := flags.String("project", "", "registered project id")
	traceID := flags.String("trace", "", "capture trace id")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *projectID == "" || *traceID == "" {
		return errors.New("inspect requires --project and --trace")
	}
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()
	result, err := application.NewInvestigationService(store).Get(context.Background(), domain.ID(*projectID), domain.ID(*traceID))
	if err != nil {
		return errors.New("trace could not be inspected")
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func splitList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	values := parts[:0]
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			values = append(values, part)
		}
	}
	return values
}

type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ",") }
func (l *stringList) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("path cannot be empty")
	}
	*l = append(*l, value)
	return nil
}
