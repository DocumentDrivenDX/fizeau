package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/agent/catalogdist"
	agentConfig "github.com/DocumentDrivenDX/agent/config"
	"github.com/DocumentDrivenDX/agent/internal/safefs"
	"github.com/DocumentDrivenDX/agent/modelcatalog"
)

const defaultCatalogBaseURL = "https://documentdrivendx.github.io/agent/catalog"

func cmdCatalog(workDir string, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: ddx-agent catalog <show|check|update|update-pricing> [flags]")
		return 2
	}
	switch args[0] {
	case "show":
		return cmdCatalogShow(workDir, args[1:])
	case "check":
		return cmdCatalogCheck(workDir, args[1:])
	case "update":
		return cmdCatalogUpdate(workDir, args[1:])
	case "update-pricing":
		return cmdCatalogUpdatePricing(workDir, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "error: unknown catalog subcommand %q\n", args[0])
		return 2
	}
}

func cmdCatalogShow(workDir string, args []string) int {
	fs := flag.NewFlagSet("catalog show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, catalog, manifestPath, err := loadCatalogRuntime(workDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}
	_ = cfg

	meta := catalog.Metadata()
	fmt.Printf("source: %s\n", meta.ManifestSource)
	fmt.Printf("manifest_path: %s\n", manifestPath)
	fmt.Printf("schema_version: %d\n", meta.ManifestVersion)
	fmt.Printf("catalog_version: %s\n", blankIfEmpty(meta.CatalogVersion))

	for _, ref := range []string{"code-high", "code-medium", "code-economy"} {
		fmt.Printf("%s:\n", ref)
		printResolvedSurface(catalog, ref, modelcatalog.SurfaceAgentAnthropic)
		printResolvedSurface(catalog, ref, modelcatalog.SurfaceAgentOpenAI)
	}
	return 0
}

func cmdCatalogCheck(workDir string, args []string) int {
	fs := flag.NewFlagSet("catalog check", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	baseURL := fs.String("base-url", defaultCatalogBaseURL, "Published catalog base URL")
	channel := fs.String("channel", "stable", "Channel to inspect")
	version := fs.String("version", "", "Exact catalog version to inspect")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	_, catalog, _, err := loadCatalogRuntime(workDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}
	local := catalog.Metadata()

	index, _, err := fetchCatalogIndex(*baseURL, *channel, *version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	fmt.Printf("local_catalog_version: %s\n", blankIfEmpty(local.CatalogVersion))
	fmt.Printf("local_source: %s\n", local.ManifestSource)
	fmt.Printf("remote_catalog_version: %s\n", index.CatalogVersion)
	fmt.Printf("remote_schema_version: %d\n", index.SchemaVersion)
	fmt.Printf("remote_published_at: %s\n", index.PublishedAt)

	if local.CatalogVersion == index.CatalogVersion && local.CatalogVersion != "" {
		fmt.Println("status: up-to-date")
		return 0
	}
	fmt.Println("status: update-available")
	return 0
}

func cmdCatalogUpdate(workDir string, args []string) int {
	fs := flag.NewFlagSet("catalog update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	baseURL := fs.String("base-url", defaultCatalogBaseURL, "Published catalog base URL")
	channel := fs.String("channel", "stable", "Channel to install")
	version := fs.String("version", "", "Exact catalog version to install")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, _, manifestPath, err := loadCatalogRuntime(workDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}
	if cfg == nil {
		cfg = &agentConfig.Config{}
	}

	index, indexURL, err := fetchCatalogIndex(*baseURL, *channel, *version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}
	manifestURL, err := resolveRelativeURL(indexURL, index.ManifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}
	manifestData, err := fetchURL(manifestURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}
	sum := sha256.Sum256(manifestData)
	if got := hex.EncodeToString(sum[:]); !strings.EqualFold(got, index.ManifestSHA256) {
		fmt.Fprintf(os.Stderr, "error: checksum mismatch: expected %s got %s\n", index.ManifestSHA256, got)
		return 1
	}

	if err := safefs.MkdirAll(filepath.Dir(manifestPath), 0o750); err != nil {
		fmt.Fprintf(os.Stderr, "error: create manifest directory: %v\n", err)
		return 1
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(manifestPath), "models-*.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: create temp manifest: %v\n", err)
		return 1
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	if _, err := tmpFile.Write(manifestData); err != nil {
		if closeErr := tmpFile.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "error: close temp manifest after write failure: %v\n", closeErr)
		}
		fmt.Fprintf(os.Stderr, "error: write temp manifest: %v\n", err)
		return 1
	}
	if err := tmpFile.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "error: close temp manifest: %v\n", err)
		return 1
	}

	if _, err := modelcatalog.Load(modelcatalog.LoadOptions{
		ManifestPath:    tmpPath,
		RequireExternal: true,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "error: validate downloaded manifest: %v\n", err)
		return 1
	}

	if err := os.Rename(tmpPath, manifestPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: install manifest: %v\n", err)
		return 1
	}

	fmt.Printf("installed catalog %s to %s\n", index.CatalogVersion, manifestPath)
	return 0
}

func cmdCatalogUpdatePricing(_ string, args []string) int {
	fs := flag.NewFlagSet("catalog update-pricing", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: resolve config dir: %v\n", err)
		return 1
	}
	manifestPath := filepath.Join(configDir, "agent", "models.yaml")

	updated, notFound, err := modelcatalog.UpdateManifestPricing(manifestPath, 15*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	fmt.Printf("Updated %d model(s) → %s\n", updated, manifestPath)
	if len(notFound) > 0 {
		fmt.Printf("Not found on OpenRouter: %s\n", strings.Join(notFound, ", "))
	}
	return 0
}

func loadCatalogRuntime(workDir string) (*agentConfig.Config, *modelcatalog.Catalog, string, error) {
	cfg, err := agentConfig.Load(workDir)
	if err != nil {
		return nil, nil, "", err
	}
	catalog, err := cfg.LoadModelCatalog()
	if err != nil {
		return nil, nil, "", err
	}
	return cfg, catalog, catalogManifestPath(cfg), nil
}

func catalogManifestPath(cfg *agentConfig.Config) string {
	if cfg != nil && strings.TrimSpace(cfg.ModelCatalog.Manifest) != "" {
		return cfg.ModelCatalog.Manifest
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(".config", "agent", "models.yaml")
	}
	return filepath.Join(configDir, "agent", "models.yaml")
}

func printResolvedSurface(catalog *modelcatalog.Catalog, ref string, surface modelcatalog.Surface) {
	resolved, err := catalog.Resolve(ref, modelcatalog.ResolveOptions{Surface: surface})
	if err != nil {
		var missingSurfaceErr *modelcatalog.MissingSurfaceError
		if errors.As(err, &missingSurfaceErr) {
			return
		}
		fmt.Printf("  %s: error=%s\n", surface, err)
		return
	}
	line := fmt.Sprintf("  %s: %s", surface, resolved.ConcreteModel)
	if resolved.SurfacePolicy.EffortDefault != "" {
		line += fmt.Sprintf(" (effort %s)", resolved.SurfacePolicy.EffortDefault)
	}
	fmt.Println(line)
}

func fetchCatalogIndex(baseURL, channel, version string) (catalogdist.Index, string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return catalogdist.Index{}, "", fmt.Errorf("catalog base URL is required")
	}
	channel = strings.TrimSpace(channel)
	version = strings.TrimSpace(version)

	indexPath := "stable/index.json"
	if version != "" {
		indexPath = path.Join("versions", version, "index.json")
	} else if channel != "" {
		indexPath = path.Join(channel, "index.json")
	}

	indexURL := baseURL + "/" + indexPath
	data, err := fetchURL(indexURL)
	if err != nil {
		return catalogdist.Index{}, "", err
	}
	var index catalogdist.Index
	if err := json.Unmarshal(data, &index); err != nil {
		return catalogdist.Index{}, "", fmt.Errorf("decode catalog index %s: %w", indexURL, err)
	}
	return index, indexURL, nil
}

func fetchURL(rawURL string) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(rawURL) // #nosec G107 -- URL is explicit operator input for catalog sync commands.
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("fetch %s: status %d: %s", rawURL, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", rawURL, err)
	}
	return data, nil
}

func resolveRelativeURL(baseURL, rel string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(rel)
	if err != nil {
		return "", err
	}
	return u.ResolveReference(ref).String(), nil
}

func blankIfEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(none)"
	}
	return s
}
