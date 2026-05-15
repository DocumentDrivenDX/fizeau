package agentcli

import (
	"path/filepath"
	"testing"

	agentConfig "github.com/easel/fizeau/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestCLIPathHelpersUseConfigPackagePaths(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	assert.Equal(t, agentConfig.ProjectRouteStateCounterPath(workDir, "main"), routeStateFile(workDir, "main"))
	assert.Equal(t, filepath.Join(workDir, agentConfig.DefaultSessionLogDir()), sessionLogDir(workDir, nil))
	assert.Equal(t, filepath.Join(home, ".config", agentConfig.GlobalConfigDirName(), "models.yaml"), catalogManifestPath(nil))
}
