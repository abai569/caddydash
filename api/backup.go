package api

import (
	"archive/zip"
	"bytes"
	"caddydash/apic"
	"caddydash/config"
	"caddydash/db"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/infinite-iroha/touka"
)

const (
	backupFormat  = "caddydash-backup"
	backupVersion = 1
)

type backupManifest struct {
	Format    string `json:"format"`
	Version   int    `json:"version"`
	CreatedAt string `json:"created_at"`
}

var backupMu sync.Mutex

func DownloadBackup(cdb *db.ConfigDB, cfg *config.Config, cfgfile string) touka.HandlerFunc {
	return func(c *touka.Context) {
		backupMu.Lock()
		defer backupMu.Unlock()

		if _, err := cdb.DB.Exec("PRAGMA wal_checkpoint(TRUNCATE);"); err != nil {
			c.Warnf("backup: wal checkpoint failed: %v", err)
		}

		caddyDir := cfg.Server.CaddyDir
		configD := filepath.Join(caddyDir, "config.d")

		buf := &bytes.Buffer{}
		zw := zip.NewWriter(buf)
		defer zw.Close()

		writeZipFile := func(name string, data []byte) error {
			fw, err := zw.Create(name)
			if err != nil {
				return err
			}
			_, err = fw.Write(data)
			if err != nil {
				return err
			}
			return nil
		}

		manifest := backupManifest{
			Format:    backupFormat,
			Version:   backupVersion,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}
		manifestJSON, err := marshalJSON(manifest)
		if err != nil {
			c.JSON(500, touka.H{"error": err.Error()})
			return
		}
		if err := writeZipFile("manifest.json", manifestJSON); err != nil {
			c.JSON(500, touka.H{"error": err.Error()})
			return
		}

		panelCfgData, err := os.ReadFile(cfgfile)
		if err != nil {
			c.JSON(500, touka.H{"error": fmt.Sprintf("read config: %v", err)})
			return
		}
		if err := writeZipFile("panel/config.toml", panelCfgData); err != nil {
			c.JSON(500, touka.H{"error": err.Error()})
			return
		}

		dbPath := cfg.DB.Filepath
		dbData, err := os.ReadFile(dbPath)
		if err != nil {
			c.JSON(500, touka.H{"error": fmt.Sprintf("read db: %v", err)})
			return
		}
		if err := writeZipFile("panel/database.db", dbData); err != nil {
			c.JSON(500, touka.H{"error": err.Error()})
			return
		}

		for _, suffix := range []string{"-wal", "-shm"} {
			p := dbPath + suffix
			if data, err := os.ReadFile(p); err == nil {
				writeZipFile("panel/database.db"+suffix, data)
			}
		}

		caddyfileData, err := os.ReadFile(filepath.Join(caddyDir, "Caddyfile"))
		if err != nil {
			c.JSON(500, touka.H{"error": fmt.Sprintf("read caddyfile: %v", err)})
			return
		}
		if err := writeZipFile("caddy/Caddyfile", caddyfileData); err != nil {
			c.JSON(500, touka.H{"error": err.Error()})
			return
		}

		entries, err := os.ReadDir(configD)
		if err == nil {
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				data, err := os.ReadFile(filepath.Join(configD, e.Name()))
				if err != nil {
					continue
				}
				writeZipFile("caddy/config.d/"+e.Name(), data)
			}
		}

		if err := zw.Close(); err != nil {
			c.JSON(500, touka.H{"error": err.Error()})
			return
		}

		ts := time.Now().UTC().Format("20060102-150405")
		c.Writer.Header().Set("Content-Type", "application/zip")
		c.Writer.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="caddydash-backup-%s.zip"`, ts))
		c.Writer.Header().Set("Content-Length", fmt.Sprintf("%d", buf.Len()))
		c.Writer.WriteHeader(http.StatusOK)
		c.Writer.Write(buf.Bytes())
	}
}

func RestoreBackup(cdb *db.ConfigDB, cfg *config.Config, cfgfile string) touka.HandlerFunc {
	return func(c *touka.Context) {
		backupMu.Lock()
		defer backupMu.Unlock()

		uploadedFile, _, err := c.Request.FormFile("backup")
		if err != nil {
			c.JSON(400, touka.H{"error": "upload failed: " + err.Error()})
			return
		}
		defer uploadedFile.Close()

		tempZip, err := os.CreateTemp("", "caddydash-restore-*.zip")
		if err != nil {
			c.JSON(500, touka.H{"error": err.Error()})
			return
		}
		tempZipPath := tempZip.Name()
		tempZip.Close()
		defer os.Remove(tempZipPath)

		if _, err := io.Copy(tempZip, uploadedFile); err != nil {
			os.Remove(tempZipPath)
			c.JSON(500, touka.H{"error": err.Error()})
			return
		}

		entries, err := readZipEntries(tempZipPath)
		if err != nil {
			c.JSON(400, touka.H{"error": err.Error()})
			return
		}

		hasConfig := false
		hasDB := false
		hasCaddyfile := false
		var cfgBytes, dbBytes, caddyfileBytes []byte
		caddyConfigFiles := make(map[string][]byte)

		for name, data := range entries {
			switch name {
			case "panel/config.toml":
				hasConfig = true
				cfgBytes = data
			case "panel/database.db":
				hasDB = true
				dbBytes = data
			case "caddy/Caddyfile":
				hasCaddyfile = true
				caddyfileBytes = data
			default:
				if strings.HasPrefix(name, "caddy/config.d/") {
					base := strings.TrimPrefix(name, "caddy/config.d/")
					if base != "" && !strings.Contains(base, "/") {
						caddyConfigFiles[base] = data
					}
				}
			}
		}

		if !hasConfig || !hasDB || !hasCaddyfile {
			c.JSON(400, touka.H{"error": "backup archive is incomplete"})
			return
		}

		var restoredCfg config.Config
		if err := toml.Unmarshal(cfgBytes, &restoredCfg); err != nil {
			c.JSON(400, touka.H{"error": "invalid config: " + err.Error()})
			return
		}

		targetCaddyDir := restoredCfg.Server.CaddyDir
		if targetCaddyDir == "" {
			targetCaddyDir = "./"
		}
		targetDBPath := restoredCfg.DB.Filepath
		if targetDBPath == "" {
			targetDBPath = cfg.DB.Filepath
		}

		targetCaddyfile := filepath.Join(targetCaddyDir, "Caddyfile")
		targetConfigD := filepath.Join(targetCaddyDir, "config.d")

		for fn, data := range caddyConfigFiles {
			if err := os.MkdirAll(targetConfigD, 0755); err != nil {
				c.JSON(500, touka.H{"error": err.Error()})
				return
			}
			if err := os.WriteFile(filepath.Join(targetConfigD, fn), data, 0644); err != nil {
				c.JSON(500, touka.H{"error": err.Error()})
				return
			}
		}

		if err := os.WriteFile(targetCaddyfile, caddyfileBytes, 0644); err != nil {
			c.JSON(500, touka.H{"error": err.Error()})
			return
		}

		tempDBDir := filepath.Dir(targetDBPath)
		tempDBName := "restore_tmp_" + time.Now().Format("20060102150405") + ".db"
		tempDBPath := filepath.Join(tempDBDir, tempDBName)

		if err := os.WriteFile(tempDBPath, dbBytes, 0644); err != nil {
			c.JSON(500, touka.H{"error": err.Error()})
			return
		}

		for _, suffix := range []string{"-wal", "-shm"} {
			if walData, ok := entries["panel/database.db"+suffix]; ok {
				os.WriteFile(tempDBPath+suffix, walData, 0644)
			}
		}

		if newCDB, err := db.InitDB(tempDBPath); err == nil {
			newCDB.CloseDB()
			os.Remove(tempDBPath)
			os.Remove(tempDBPath + "-wal")
			os.Remove(tempDBPath + "-shm")

			samePath := filepath.Clean(targetDBPath) == filepath.Clean(cfg.DB.Filepath)
			if samePath {
				if err := cdb.CloseDB(); err != nil {
					c.JSON(500, touka.H{"error": err.Error()})
					return
				}
			}

			if err := os.WriteFile(targetDBPath, dbBytes, 0644); err != nil {
				c.JSON(500, touka.H{"error": err.Error()})
				return
			}

			if samePath {
				newCDB, err := db.InitDB(targetDBPath)
				if err != nil {
					c.JSON(500, touka.H{"error": err.Error()})
					return
				}
				newCDB.DB.SetMaxOpenConns(1)
				newCDB.DB.SetMaxIdleConns(1)
				*cdb = *newCDB
			}
		} else {
			if err := os.Remove(tempDBPath); err != nil {
				// ignore cleanup error
			}
			c.JSON(500, touka.H{"error": err.Error()})
			return
		}

		if err := os.WriteFile(cfgfile, cfgBytes, 0644); err != nil {
			c.JSON(500, touka.H{"error": err.Error()})
			return
		}

		*cfg = restoredCfg

		if err := apic.RestartCaddyProcess(cfg); err != nil {
			c.JSON(500, touka.H{"error": "restore ok but caddy reload failed: " + err.Error()})
			return
		}

		c.JSON(200, touka.H{"message": "restored and caddy reloaded"})
	}
}

func readZipEntries(zipPath string) (map[string][]byte, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("open zip: %v", err)
	}
	defer zr.Close()

	result := make(map[string][]byte)
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		clean := path.Clean(f.Name)
		if strings.HasPrefix(clean, "../") || clean == ".." || strings.Contains(clean, "..") {
			return nil, fmt.Errorf("unsafe path: %s", f.Name)
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		result[clean] = data
	}
	return result, nil
}

func marshalJSON(v interface{}) ([]byte, error) {
	return []byte(fmt.Sprintf(`{"format":"%s","version":%d,"created_at":"%s"}`,
		backupFormat, backupVersion, time.Now().UTC().Format(time.RFC3339))), nil
}
