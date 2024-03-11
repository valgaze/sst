package telemetry

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/posthog/posthog-go"
	"github.com/sst/ion/internal/fs"
	"github.com/sst/ion/internal/util"
	"github.com/sst/ion/pkg/global"
)

const (
	TELEMETRY_DISABLED_KEY = "telemetry-disable"
	TELEMETRY_ID_KEY       = "telemetry-id"
)

func Disable() error {
	path := filepath.Join(global.ConfigDir(), TELEMETRY_DISABLED_KEY)
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	return file.Close()
}

func Enable() error {
	path := filepath.Join(global.ConfigDir(), TELEMETRY_DISABLED_KEY)
	os.Remove(path)
	return nil
}

func IsEnabled() bool {
	path := filepath.Join(global.ConfigDir(), TELEMETRY_DISABLED_KEY)
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}

// detectCI attempts to detect the CI environment and returns its name if detected, empty string otherwise.
func detectCI() (ciName string) {
	// You may need to add more CI detection logic here

	return ""
}

var telemetryEnvironment = sync.OnceValue((func() map[string]interface{} {
	telemetryIDPath := filepath.Join(global.ConfigDir(), TELEMETRY_ID_KEY)
	var userID string
	userIDBytes, err := os.ReadFile(telemetryIDPath)
	if err == nil {
		userID = string(userIDBytes)
	} else {
		userID = util.RandomString(32)
		os.WriteFile(telemetryIDPath, []byte(userID), 0600)
	}

	cwd, _ := os.Getwd()
	fs.FindUp(cwd, ".git")

	cmd := exec.Command("git", "rev-list", "--max-parents=0", "HEAD")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Run()
	rootCommitHash := out.String()
	projectID := ""
	if len(rootCommitHash) == 40 {
		projectID = rootCommitHash
	}
	sessionID := util.RandomString(32)

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	ciEnvVars := map[string]string{
		"GITHUB_ACTIONS": "GitHub Actions",
		"GITLAB_CI":      "GitLab CI",
		"CIRCLECI":       "CircleCI",
		"JENKINS_URL":    "Jenkins",
		"TRAVIS":         "Travis CI",
	}
	ci := ""
	for envVar, name := range ciEnvVars {
		if _, present := os.LookupEnv(envVar); present {
			ci = name
		}
	}
	return map[string]interface{}{
		"id": map[string]interface{}{
			"user":    userID,
			"project": projectID,
			"session": sessionID,
		},
		"system": map[string]interface{}{
			"platform":     runtime.GOOS,
			"architecture": runtime.GOARCH,
		},
		"cpu": map[string]interface{}{
			"count": runtime.NumCPU(),
		},
		"memory": map[string]interface{}{
			"total": int(memStats.TotalAlloc),
		},
		"ci": map[string]interface{}{
			"name": ci,
		},
		"sst": map[string]interface{}{
			"version": version,
		},
	}
}))

var client = (func() posthog.Client {
	client, _ := posthog.NewWithConfig("phc_M0b2lW4smpsGIufiTBZ22USKwCy0fyqljMOGufJc79p",
		posthog.Config{
			Endpoint: "https://telemetry.ion.sst.dev",
		},
	)
	return client
})()

var version = "unknown"

func SetVersion(value string) {
	version = value
}

func Track(event string, properties map[string]interface{}) {
	go func() {
		if !IsEnabled() {
			return
		}
		env := telemetryEnvironment()
		properties["environment"] = env
		userID := env["id"].(map[string]interface{})["user"]
		client.Enqueue(posthog.Capture{
			DistinctId: userID.(string),
			Event:      event,
			Properties: properties,
		})
	}()
}

func Close() {
	client.Close()
}